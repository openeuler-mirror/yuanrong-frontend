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
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testTunnelNegotiationTimeout = 10 * time.Millisecond

func TestDialTunnelUsesPlainTCPWhenMTLSIsDisabled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverErr <- acceptErr
			return
		}
		defer conn.Close()
		var header tunnelHeader
		if readErr := readFramedJSON(conn, &header); readErr != nil {
			serverErr <- readErr
			return
		}
		if writeErr := writeFramedJSON(conn, tunnelResponse{OK: true}); writeErr != nil {
			serverErr <- writeErr
			return
		}
		serverErr <- nil
	}()

	conn, err := dialTunnel(listener.Addr().String(), nil, tunnelHeader{
		TunnelVersion: tunnelVersion,
		InstanceID:    "instance-1",
		Protocol:      "tcp",
		TargetPort:    22,
		RequestID:     "request-1",
	})
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.NoError(t, <-serverErr)
}

func TestDialTunnelTimesOutWhenProxyDoesNotAnswerNegotiation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	accepted := make(chan struct{})
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		close(accepted)
		time.Sleep(time.Second)
	}()

	_, err = dialTunnelWithTimeout(listener.Addr().String(), nil, tunnelHeader{
		TunnelVersion: tunnelVersion,
		InstanceID:    "instance-1",
		Protocol:      "tcp",
		TargetPort:    22,
		RequestID:     "request-timeout",
	}, testTunnelNegotiationTimeout)

	require.Error(t, err)
	require.Contains(t, err.Error(), "read tunnel response")
	<-accepted
}

func TestLoadTunnelTLSConfigDoesNotRequireCertificatesWhenDisabled(t *testing.T) {
	config, err := loadTunnelTLSConfig(false)
	require.NoError(t, err)
	require.Nil(t, config)
}
