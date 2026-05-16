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
	"testing"
)

// helper: build a binary request frame
func buildReqFrame(version byte, op byte, id string, payload []byte) []byte {
	idBytes := []byte(id)
	buf := make([]byte, 4+len(idBytes)+len(payload))
	buf[0] = version
	buf[1] = op
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(idBytes)))
	copy(buf[4:], idBytes)
	copy(buf[4+len(idBytes):], payload)
	return buf
}

func TestParseBinaryRequest(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		wantOp      byte
		wantID      string
		wantPayload []byte
		wantErr     bool
	}{
		{
			name:        "valid create request",
			data:        buildReqFrame(BinFrameVersion, BinOpCreate, "req-001", []byte{0x08, 0x01, 0x12, 0x05}),
			wantOp:      BinOpCreate,
			wantID:      "req-001",
			wantPayload: []byte{0x08, 0x01, 0x12, 0x05},
		},
		{
			name:        "valid invoke request",
			data:        buildReqFrame(BinFrameVersion, BinOpInvoke, "uuid-1234-5678", []byte("raw-protobuf-bytes")),
			wantOp:      BinOpInvoke,
			wantID:      "uuid-1234-5678",
			wantPayload: []byte("raw-protobuf-bytes"),
		},
		{
			name:        "empty payload is valid",
			data:        buildReqFrame(BinFrameVersion, BinOpCreate, "r1", nil),
			wantOp:      BinOpCreate,
			wantID:      "r1",
			wantPayload: []byte{},
		},
		{
			name:    "frame too short",
			data:    []byte{0x01, 0x01, 0x00},
			wantErr: true,
		},
		{
			name:    "wrong version",
			data:    []byte{0x99, BinOpCreate, 0x00, 0x02, 'a', 'b'},
			wantErr: true,
		},
		{
			name:    "truncated id",
			data:    []byte{BinFrameVersion, BinOpCreate, 0x00, 0x0A, 'a', 'b'},
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
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
			if len(got.Payload) != len(tt.wantPayload) {
				t.Fatalf("Payload len = %d, want %d", len(got.Payload), len(tt.wantPayload))
			}
			for i := range tt.wantPayload {
				if got.Payload[i] != tt.wantPayload[i] {
					t.Errorf("Payload[%d] = %d, want %d", i, got.Payload[i], tt.wantPayload[i])
				}
			}
		})
	}
}

func TestBuildBinaryResponse(t *testing.T) {
	tests := []struct {
		name    string
		reqID   string
		status  byte
		payload []byte
	}{
		{
			name:    "success with payload",
			reqID:   "req-001",
			status:  BinStatusOK,
			payload: []byte{0x08, 0x01},
		},
		{
			name:    "error response",
			reqID:   "req-002",
			status:  BinStatusError,
			payload: BuildBinaryErrorPayload(1005, "operation failed"),
		},
		{
			name:    "empty payload",
			reqID:   "r",
			status:  BinStatusOK,
			payload: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := BuildBinaryResponse(tt.reqID, tt.status, tt.payload)

			if frame[0] != BinFrameVersion {
				t.Errorf("version = %d, want %d", frame[0], BinFrameVersion)
			}
			if frame[1] != tt.status {
				t.Errorf("status = %d, want %d", frame[1], tt.status)
			}
			idLen := binary.BigEndian.Uint16(frame[2:4])
			if int(idLen) != len(tt.reqID) {
				t.Errorf("id len = %d, want %d", idLen, len(tt.reqID))
			}
			gotID := string(frame[4 : 4+idLen])
			if gotID != tt.reqID {
				t.Errorf("id = %q, want %q", gotID, tt.reqID)
			}
			gotPayload := frame[4+idLen:]
			if len(gotPayload) != len(tt.payload) {
				t.Fatalf("payload len = %d, want %d", len(gotPayload), len(tt.payload))
			}
			for i := range tt.payload {
				if gotPayload[i] != tt.payload[i] {
					t.Errorf("payload[%d] = %d, want %d", i, gotPayload[i], tt.payload[i])
				}
			}
		})
	}
}

func TestBuildBinaryErrorPayload(t *testing.T) {
	code := 1005
	msg := "timeout"
	payload := BuildBinaryErrorPayload(code, msg)

	gotCode := binary.BigEndian.Uint32(payload[0:4])
	if int(gotCode) != code {
		t.Errorf("error code = %d, want %d", gotCode, code)
	}
	gotMsg := string(payload[4:])
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

	if respFrame[0] != BinFrameVersion {
		t.Errorf("response version = %d", respFrame[0])
	}
	if respFrame[1] != BinStatusOK {
		t.Errorf("response status = %d", respFrame[1])
	}
	respIDLen := binary.BigEndian.Uint16(respFrame[2:4])
	if string(respFrame[4:4+respIDLen]) != reqID {
		t.Errorf("response id mismatch")
	}
	if string(respFrame[4+respIDLen:]) != string(respPayload) {
		t.Errorf("response payload mismatch")
	}
}
