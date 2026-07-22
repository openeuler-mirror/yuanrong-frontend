/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2026. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sshproxy

import (
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	tunnelVersion        = 1
	maxTunnelHeaderSize  = 16 * 1024
	tunnelFrameSizeBytes = 4
	tunnelDialTimeout    = 10 * time.Second
	tunnelKeepAlive      = 30 * time.Second
)

type tunnelHeader struct {
	TunnelVersion int    `json:"tunnelVersion"`
	InstanceID    string `json:"instanceID"`
	Protocol      string `json:"protocol"`
	TargetPort    int    `json:"targetPort"`
	RequestID     string `json:"requestID"`
	TraceID       string `json:"traceID,omitempty"`
}

type tunnelResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

func dialTunnel(address string, tlsConfig *tls.Config, header tunnelHeader) (net.Conn, error) {
	return dialTunnelWithTimeout(address, tlsConfig, header, tunnelDialTimeout)
}

func dialTunnelWithTimeout(address string, tlsConfig *tls.Config, header tunnelHeader,
	timeout time.Duration,
) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: tunnelDialTimeout, KeepAlive: tunnelKeepAlive}
	var conn net.Conn
	var err error
	if tlsConfig == nil {
		conn, err = dialer.Dial("tcp", address)
	} else {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("dial function proxy tunnel: %w", err)
	}
	if err = conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, closeTunnelAfterError(conn, fmt.Errorf("set tunnel negotiation deadline: %w", err))
	}
	if err = writeFramedJSON(conn, header); err != nil {
		return nil, closeTunnelAfterError(conn, fmt.Errorf("send tunnel header: %w", err))
	}
	var response tunnelResponse
	if err = readFramedJSON(conn, &response); err != nil {
		return nil, closeTunnelAfterError(conn, fmt.Errorf("read tunnel response: %w", err))
	}
	if !response.OK {
		return nil, closeTunnelAfterError(conn,
			fmt.Errorf("function proxy rejected tunnel: %s", response.Message))
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		return nil, closeTunnelAfterError(conn, fmt.Errorf("clear tunnel negotiation deadline: %w", err))
	}
	return conn, nil
}

func writeFramedJSON(writer io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > maxTunnelHeaderSize {
		return fmt.Errorf("invalid framed JSON size %d", len(payload))
	}
	var size [tunnelFrameSizeBytes]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(payload)))
	if _, err = writer.Write(size[:]); err != nil {
		return err
	}
	_, err = writer.Write(payload)
	return err
}

func readFramedJSON(reader io.Reader, value any) error {
	var size [tunnelFrameSizeBytes]byte
	if _, err := io.ReadFull(reader, size[:]); err != nil {
		return err
	}
	length := binary.BigEndian.Uint32(size[:])
	if length == 0 || length > maxTunnelHeaderSize {
		return fmt.Errorf("invalid framed JSON size %d", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return err
	}
	return json.Unmarshal(payload, value)
}

func closeTunnelAfterError(conn net.Conn, cause error) error {
	if err := conn.Close(); err != nil {
		return fmt.Errorf("%w; close tunnel: %v", cause, err)
	}
	return cause
}
