/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2025. All rights reserved.
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

package posixws

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// MessageType defines the type of WebSocket message
type MessageType string

const (
	MessageTypeRequest  MessageType = "request"
	MessageTypeResponse MessageType = "response"
	MessageTypePing     MessageType = "ping"
	MessageTypePong     MessageType = "pong"
	MessageTypeClose    MessageType = "close"
)

// OperationType defines the type of operation
type OperationType string

const (
	OperationCreate OperationType = "create"
	OperationInvoke OperationType = "invoke"
)

// StatusType defines the status of response
type StatusType string

const (
	StatusSuccess StatusType = "success"
	StatusError   StatusType = "error"
)

// RequestMessage represents a client request message
type RequestMessage struct {
	Type      MessageType       `json:"type"`
	ID        string            `json:"id"`
	Operation OperationType     `json:"operation"`
	Payload   string            `json:"payload"` // Base64-encoded protobuf
	Headers   map[string]string `json:"headers,omitempty"`
}

// ResponseMessage represents a server response message
type ResponseMessage struct {
	Type    MessageType `json:"type"`
	ID      string      `json:"id"`
	Status  StatusType  `json:"status"`
	Payload string      `json:"payload,omitempty"` // Base64-encoded protobuf
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo represents error information in response
type ErrorInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *ErrorInfo) Error() string {
	return e.Message
}

// ControlMessage represents ping/pong/close control messages
type ControlMessage struct {
	Type      MessageType `json:"type"`
	Timestamp int64       `json:"timestamp,omitempty"`
}

// ParseRequestMessage parses JSON bytes into RequestMessage
func ParseRequestMessage(data []byte) (*RequestMessage, error) {
	var msg RequestMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ToJSON serializes ResponseMessage to JSON bytes
func (m *ResponseMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// ToJSON serializes ControlMessage to JSON bytes
func (m *ControlMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// NewSuccessResponse creates a success response message
func NewSuccessResponse(id string, payload string) *ResponseMessage {
	return &ResponseMessage{
		Type:    MessageTypeResponse,
		ID:      id,
		Status:  StatusSuccess,
		Payload: payload,
	}
}

// NewErrorResponse creates an error response message
func NewErrorResponse(id string, code int, message string) *ResponseMessage {
	return &ResponseMessage{
		Type:   MessageTypeResponse,
		ID:     id,
		Status: StatusError,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	}
}

// NewPongMessage creates a pong control message
func NewPongMessage(timestamp int64) *ControlMessage {
	return &ControlMessage{
		Type:      MessageTypePong,
		Timestamp: timestamp,
	}
}

// Binary frame protocol constants
const (
	BinFrameVersion = 0x01
	BinOpCreate     = 0x01
	BinOpInvoke     = 0x02
	BinStatusOK     = 0x00
	BinStatusError  = 0x01
)

// BinaryRequest represents a parsed binary frame request
type BinaryRequest struct {
	Operation byte
	ID        string
	Payload   []byte
}

// ParseBinaryRequest parses a binary frame: [version][op][id_len_be][id][payload]
func ParseBinaryRequest(data []byte) (*BinaryRequest, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("binary frame too short: %d bytes", len(data))
	}
	if data[0] != BinFrameVersion {
		return nil, fmt.Errorf("unknown binary frame version: %d", data[0])
	}
	opCode := data[1]
	idLen := binary.BigEndian.Uint16(data[2:4])
	if len(data) < 4+int(idLen) {
		return nil, fmt.Errorf("binary frame truncated: need %d, got %d", 4+int(idLen), len(data))
	}
	return &BinaryRequest{
		Operation: opCode,
		ID:        string(data[4 : 4+idLen]),
		Payload:   data[4+idLen:],
	}, nil
}

// BuildBinaryResponse builds a binary response frame: [version][status][id_len_be][id][payload]
func BuildBinaryResponse(reqID string, status byte, payload []byte) []byte {
	idBytes := []byte(reqID)
	buf := make([]byte, 4+len(idBytes)+len(payload))
	buf[0] = BinFrameVersion
	buf[1] = status
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(idBytes)))
	copy(buf[4:], idBytes)
	copy(buf[4+len(idBytes):], payload)
	return buf
}

// BuildBinaryErrorPayload builds the error payload: [4B error code BE][error message]
func BuildBinaryErrorPayload(code int, message string) []byte {
	msgBytes := []byte(message)
	buf := make([]byte, 4+len(msgBytes))
	binary.BigEndian.PutUint32(buf[0:4], uint32(code))
	copy(buf[4:], msgBytes)
	return buf
}
