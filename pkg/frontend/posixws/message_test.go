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
	frame := buildReqFrame(BinFrameVersion, BinOpInvoke, "req-1", []byte("payload"))

	req, err := ParseBinaryRequest(frame)
	if err != nil {
		t.Fatalf("ParseBinaryRequest() error = %v", err)
	}
	if req.Operation != BinOpInvoke {
		t.Fatalf("Operation = %d, want %d", req.Operation, BinOpInvoke)
	}
	if req.ID != "req-1" {
		t.Fatalf("ID = %q, want req-1", req.ID)
	}
	if string(req.Payload) != "payload" {
		t.Fatalf("Payload = %q, want payload", req.Payload)
	}
}

func TestBuildBinaryErrorPayload(t *testing.T) {
	payload := BuildBinaryErrorPayload(1005, "timeout")

	if got := binary.BigEndian.Uint32(payload[0:4]); got != 1005 {
		t.Fatalf("error code = %d, want 1005", got)
	}
	if got := string(payload[4:]); got != "timeout" {
		t.Fatalf("error message = %q, want timeout", got)
	}
}
