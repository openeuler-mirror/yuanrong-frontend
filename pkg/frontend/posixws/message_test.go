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
	"bytes"
	"encoding/binary"
	"testing"
)

const (
	testErrorCode       = 1005
	testInvalidIDLength = 10
)

// helper: build a binary request frame
func buildReqFrame(version byte, op byte, id string, payload []byte) []byte {
	idBytes := []byte(id)
	buf := make([]byte, binaryHeaderLen+len(idBytes)+len(payload))
	buf[binaryVersionOffset] = version
	buf[binaryOpOffset] = op
	binary.BigEndian.PutUint16(buf[binaryIDLenOffset:binaryPayloadOffset], uint16(len(idBytes)))
	copy(buf[binaryHeaderLen:], idBytes)
	copy(buf[binaryHeaderLen+len(idBytes):], payload)
	return buf
}

type parseBinaryRequestCase struct {
	name        string
	data        []byte
	wantOp      byte
	wantID      string
	wantPayload []byte
	wantErr     bool
}

func parseBinaryRequestCases() []parseBinaryRequestCase {
	protobufPayload := []byte{0x08, 0x01, 0x12, 0x05}
	return []parseBinaryRequestCase{
		{name: "valid create request", data: buildReqFrame(BinFrameVersion, BinOpCreate, "req-001", protobufPayload),
			wantOp: BinOpCreate, wantID: "req-001", wantPayload: protobufPayload},
		{name: "valid invoke request",
			data:   buildReqFrame(BinFrameVersion, BinOpInvoke, "uuid-1234-5678", []byte("raw-protobuf-bytes")),
			wantOp: BinOpInvoke, wantID: "uuid-1234-5678", wantPayload: []byte("raw-protobuf-bytes")},
		{name: "empty payload is valid", data: buildReqFrame(BinFrameVersion, BinOpCreate, "r1", nil),
			wantOp: BinOpCreate, wantID: "r1", wantPayload: []byte{}},
		{name: "frame too short", data: []byte{BinFrameVersion, BinOpCreate, 0x00}, wantErr: true},
		{name: "wrong version", data: []byte{0x99, BinOpCreate, 0x00, 0x02, 'a', 'b'}, wantErr: true},
		{name: "truncated id", data: []byte{BinFrameVersion, BinOpCreate, 0x00, testInvalidIDLength, 'a', 'b'},
			wantErr: true},
		{name: "empty data", data: []byte{}, wantErr: true},
	}
}

func TestParseBinaryRequest(t *testing.T) {
	for _, tt := range parseBinaryRequestCases() {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBinaryRequest(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Operation != tt.wantOp {
				t.Errorf("Operation = %d, want %d", got.Operation, tt.wantOp)
			}
			if got.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tt.wantID)
			}
			if !bytes.Equal(got.Payload, tt.wantPayload) {
				t.Errorf("Payload = %v, want %v", got.Payload, tt.wantPayload)
			}
		})
	}
}

type buildBinaryResponseCase struct {
	name    string
	reqID   string
	status  byte
	payload []byte
}

func buildBinaryResponseCases() []buildBinaryResponseCase {
	return []buildBinaryResponseCase{
		{name: "success with payload", reqID: "req-001", status: BinStatusOK, payload: []byte{0x08, 0x01}},
		{name: "error response", reqID: "req-002", status: BinStatusError,
			payload: BuildBinaryErrorPayload(testErrorCode, "operation failed")},
		{name: "empty payload", reqID: "r", status: BinStatusOK, payload: []byte{}},
	}
}

func TestBuildBinaryResponse(t *testing.T) {
	for _, tt := range buildBinaryResponseCases() {
		t.Run(tt.name, func(t *testing.T) {
			frame := BuildBinaryResponse(tt.reqID, tt.status, tt.payload)

			if frame[binaryVersionOffset] != BinFrameVersion {
				t.Errorf("version = %d, want %d", frame[binaryVersionOffset], BinFrameVersion)
			}
			if frame[binaryOpOffset] != tt.status {
				t.Errorf("status = %d, want %d", frame[binaryOpOffset], tt.status)
			}
			idLen := binary.BigEndian.Uint16(frame[binaryIDLenOffset:binaryPayloadOffset])
			if int(idLen) != len(tt.reqID) {
				t.Errorf("id len = %d, want %d", idLen, len(tt.reqID))
			}
			payloadStart := binaryHeaderLen + int(idLen)
			gotID := string(frame[binaryHeaderLen:payloadStart])
			if gotID != tt.reqID {
				t.Errorf("id = %q, want %q", gotID, tt.reqID)
			}
			if gotPayload := frame[payloadStart:]; !bytes.Equal(gotPayload, tt.payload) {
				t.Errorf("payload = %v, want %v", gotPayload, tt.payload)
			}
		})
	}
}

func TestBuildBinaryErrorPayload(t *testing.T) {
	code := testErrorCode
	msg := "timeout"
	payload := BuildBinaryErrorPayload(code, msg)

	gotCode := binary.BigEndian.Uint32(payload[:binaryHeaderLen])
	if int(gotCode) != code {
		t.Errorf("error code = %d, want %d", gotCode, code)
	}
	gotMsg := string(payload[binaryHeaderLen:])
	if gotMsg != msg {
		t.Errorf("error message = %q, want %q", gotMsg, msg)
	}
}

func TestRoundTrip(t *testing.T) {
	reqID := "roundtrip-test-id"
	opCode := byte(BinOpInvoke)
	payload := []byte("hello-protobuf-world")

	// Build request frame (same layout as C++ client)
	frame := buildReqFrame(BinFrameVersion, opCode, reqID, payload)

	// Parse it
	req, err := ParseBinaryRequest(frame)
	if err != nil {
		t.Fatalf("ParseBinaryRequest() error: %v", err)
	}
	if req.Operation != opCode {
		t.Errorf("Operation = %d, want %d", req.Operation, opCode)
	}
	if req.ID != reqID {
		t.Errorf("ID = %q, want %q", req.ID, reqID)
	}
	if string(req.Payload) != string(payload) {
		t.Errorf("Payload = %q, want %q", req.Payload, payload)
	}

	// Build response and verify structure
	respPayload := []byte("response-protobuf")
	respFrame := BuildBinaryResponse(reqID, BinStatusOK, respPayload)

	if respFrame[binaryVersionOffset] != BinFrameVersion {
		t.Errorf("response version = %d", respFrame[binaryVersionOffset])
	}
	if respFrame[binaryOpOffset] != BinStatusOK {
		t.Errorf("response status = %d", respFrame[binaryOpOffset])
	}
	respIDLen := binary.BigEndian.Uint16(respFrame[binaryIDLenOffset:binaryPayloadOffset])
	payloadStart := binaryHeaderLen + int(respIDLen)
	if string(respFrame[binaryHeaderLen:payloadStart]) != reqID {
		t.Errorf("response id mismatch")
	}
	if !bytes.Equal(respFrame[payloadStart:], respPayload) {
		t.Errorf("response payload mismatch")
	}
}
