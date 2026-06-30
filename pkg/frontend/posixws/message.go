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
	// MessageTypeRequest identifies a client request frame.
	MessageTypeRequest MessageType = "request"
	// MessageTypeResponse identifies a server response frame.
	MessageTypeResponse MessageType = "response"
	// MessageTypePing identifies a ping control frame.
	MessageTypePing MessageType = "ping"
	// MessageTypePong identifies a pong control frame.
	MessageTypePong MessageType = "pong"
	// MessageTypeClose identifies a close control frame.
	MessageTypeClose MessageType = "close"
)

// OperationType defines the type of operation
type OperationType string

const (
	// OperationCreate creates a sandbox function instance.
	OperationCreate OperationType = "create"
	// OperationInvoke invokes an existing sandbox function instance.
	OperationInvoke OperationType = "invoke"
)

// StatusType defines the status of response
type StatusType string

const (
	// StatusSuccess marks a successful response.
	StatusSuccess StatusType = "success"
	// StatusError marks a failed response.
	StatusError StatusType = "error"
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
	Type    MessageType    `json:"type"`
	ID      string         `json:"id"`
	Status  StatusType     `json:"status"`
	Payload string         `json:"payload,omitempty"` // Base64-encoded protobuf
	Error   *ResponseError `json:"error,omitempty"`
}

// ResponseError represents error information in response.
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error returns the response error message.
func (e *ResponseError) Error() string {
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
		Error: &ResponseError{
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
	// BinFrameVersion is the supported binary frame protocol version.
	BinFrameVersion = 0x01
	// BinOpCreate identifies a create request frame.
	BinOpCreate = 0x01
	// BinOpInvoke identifies an invoke request frame.
	BinOpInvoke = 0x02
	// BinStatusOK identifies a successful response frame.
	BinStatusOK = 0x00
	// BinStatusError identifies an error response frame.
	BinStatusError = 0x01
)

const (
	binaryVersionOffset = 0
	binaryOpOffset      = 1
	binaryIDLenOffset   = 2
	binaryPayloadOffset = 4
	binaryHeaderLen     = 4
)

// BinaryRequest represents a parsed binary frame request
type BinaryRequest struct {
	Operation byte
	ID        string
	Payload   []byte
}

// ParseBinaryRequest parses a binary frame: [version][op][id_len_be][id][payload]
func ParseBinaryRequest(data []byte) (*BinaryRequest, error) {
	if len(data) < binaryHeaderLen {
		return nil, fmt.Errorf("binary frame too short: %d bytes", len(data))
	}
	if data[binaryVersionOffset] != BinFrameVersion {
		return nil, fmt.Errorf("unknown binary frame version: %d", data[binaryVersionOffset])
	}
	opCode := data[binaryOpOffset]
	idLen := binary.BigEndian.Uint16(data[binaryIDLenOffset:binaryPayloadOffset])
	payloadStart := binaryHeaderLen + int(idLen)
	if len(data) < payloadStart {
		return nil, fmt.Errorf("binary frame truncated: need %d, got %d", payloadStart, len(data))
	}
	return &BinaryRequest{
		Operation: opCode,
		ID:        string(data[binaryHeaderLen:payloadStart]),
		Payload:   data[payloadStart:],
	}, nil
}

// BuildBinaryResponse builds a binary response frame: [version][status][id_len_be][id][payload]
func BuildBinaryResponse(reqID string, status byte, payload []byte) []byte {
	idBytes := []byte(reqID)
	buf := make([]byte, binaryHeaderLen+len(idBytes)+len(payload))
	buf[binaryVersionOffset] = BinFrameVersion
	buf[binaryOpOffset] = status
	binary.BigEndian.PutUint16(buf[binaryIDLenOffset:binaryPayloadOffset], uint16(len(idBytes)))
	copy(buf[binaryHeaderLen:], idBytes)
	copy(buf[binaryHeaderLen+len(idBytes):], payload)
	return buf
}

// BuildBinaryErrorPayload builds the error payload: [4B error code BE][error message]
func BuildBinaryErrorPayload(code int, message string) []byte {
	msgBytes := []byte(message)
	buf := make([]byte, binaryHeaderLen+len(msgBytes))
	binary.BigEndian.PutUint32(buf[:binaryHeaderLen], uint32(code))
	copy(buf[binaryHeaderLen:], msgBytes)
	return buf
}
