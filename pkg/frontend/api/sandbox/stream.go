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

package sandbox

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/common/util"
	api "yuanrong.org/kernel/runtime/libruntime/api"
)

const (
	streamProtocolVersion = "sandbox.stream.v1"
	streamBinaryMagic     = "YRS1"
	defaultStreamChunk    = 64 * 1024
	maxStreamChunk        = 4 * 1024 * 1024
	streamInvokeTimeout   = 300
	streamPongWait        = 60 * time.Second
)

var sandboxStreamUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

type streamControlFrame struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Path      string `json:"path,omitempty"`
	Mode      string `json:"mode,omitempty"`
	ChunkSize int    `json:"chunkSize,omitempty"`
	// StreamType selects the runtime stream payload: "file" (default) for a
	// single file, or "tar" for directory transfer (the payload is a tar
	// archive the runtime extracts on upload / produces on download).
	StreamType string `json:"streamType,omitempty"`
}

// streamTypeOrFile defaults an empty stream payload type to "file" so older
// clients (no streamType field) keep the single-file behaviour.
func streamTypeOrFile(t string) string {
	if t == "" {
		return "file"
	}
	return t
}

type streamResponseFrame struct {
	ID           string                 `json:"id,omitempty"`
	Type         string                 `json:"type"`
	Protocol     string                 `json:"protocol,omitempty"`
	SandboxID    string                 `json:"sandboxId,omitempty"`
	StreamID     string                 `json:"streamId,omitempty"`
	Path         string                 `json:"path,omitempty"`
	Offset       int64                  `json:"offset,omitempty"`
	Bytes        int64                  `json:"bytes,omitempty"`
	BytesWritten int64                  `json:"bytesWritten,omitempty"`
	BytesRead    int64                  `json:"bytesRead,omitempty"`
	EOF          bool                   `json:"eof,omitempty"`
	Code         string                 `json:"code,omitempty"`
	Message      string                 `json:"message,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
}

type uploadState struct {
	path     string
	streamID string
	offset   int64
}

// StreamV1Handler handles GET /api/sandbox/v1/sandboxes/{sandboxID}/stream.
//
// Protocol v1 intentionally keeps HTTP create/delete/invoke as the control
// plane and uses this WebSocket only as the streaming data plane. Text frames
// are JSON control messages; binary frames use:
//
//	[4B magic "YRS1"][2B big-endian op-id length][op-id bytes][payload bytes]
//
// Supported first-version operations:
//   - file.upload.start / binary chunks / file.upload.finish
//   - file.download.start -> binary chunks + file.download.done
//
// File streams use native RRT sandbox_stream_* leases. Interactive
// shell/process streaming are reserved protocol types and return a clear
// "not implemented" error until RRT exposes matching streaming leases.
func StreamV1Handler(ctx *gin.Context) {
	sandboxID := ctx.Param("sandboxID")
	if sandboxID == "" {
		sandboxID = ctx.Param("sandboxId")
	}
	if sandboxID == "" {
		ctx.String(http.StatusBadRequest, "sandboxID is required")
		return
	}

	var upgradeHeader http.Header
	if protos := websocket.Subprotocols(ctx.Request); len(protos) > 0 {
		upgradeHeader = http.Header{"Sec-WebSocket-Protocol": []string{protos[0]}}
	}
	conn, err := sandboxStreamUpgrader.Upgrade(ctx.Writer, ctx.Request, upgradeHeader)
	if err != nil {
		log.GetLogger().Errorf("sandbox stream websocket upgrade failed sandboxID=%s: %v", sandboxID, err)
		return
	}
	defer conn.Close()

	session := &sandboxStreamSession{
		conn:      conn,
		sandboxID: sandboxID,
		uploads:   map[string]*uploadState{},
	}
	session.send(streamResponseFrame{
		Type:      "hello",
		Protocol:  streamProtocolVersion,
		SandboxID: sandboxID,
	})
	session.run()
}

type sandboxStreamSession struct {
	conn      *websocket.Conn
	sandboxID string
	uploads   map[string]*uploadState
	writeMu   sync.Mutex
}

func (s *sandboxStreamSession) run() {
	defer s.cleanupUploads()

	_ = s.conn.SetReadDeadline(time.Now().Add(streamPongWait))
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(streamPongWait))
	})
	s.conn.SetPingHandler(func(appData string) error {
		_ = s.conn.SetReadDeadline(time.Now().Add(streamPongWait))
		return s.conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})

	for {
		messageType, payload, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.GetLogger().Warnf("sandbox stream websocket closed unexpectedly sandboxID=%s: %v", s.sandboxID, err)
			}
			return
		}
		_ = s.conn.SetReadDeadline(time.Now().Add(streamPongWait))

		switch messageType {
		case websocket.TextMessage:
			s.handleControl(payload)
		case websocket.BinaryMessage:
			s.handleBinary(payload)
		default:
			s.sendError("", "UnsupportedFrame", fmt.Sprintf("unsupported websocket message type %d", messageType), nil)
		}
	}
}

func (s *sandboxStreamSession) handleControl(payload []byte) {
	var frame streamControlFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		s.sendError("", "InvalidFrame", fmt.Sprintf("invalid JSON control frame: %v", err), nil)
		return
	}
	switch frame.Type {
	case "ping":
		s.send(streamResponseFrame{ID: frame.ID, Type: "pong"})
	case "file.upload.start":
		s.startUpload(frame)
	case "file.upload.finish":
		s.finishUpload(frame)
	case "file.download.start":
		s.download(frame)
	case "shell.open", "shell.stdin", "process.start", "process.stdin":
		s.sendError(frame.ID, "NotImplemented", "interactive shell/process streaming requires the next RRT stream lease implementation", nil)
	default:
		s.sendError(frame.ID, "UnsupportedOperation", fmt.Sprintf("unsupported stream operation %q", frame.Type), nil)
	}
}

func (s *sandboxStreamSession) startUpload(frame streamControlFrame) {
	if frame.ID == "" {
		s.sendError("", "InvalidFrame", "id is required for file.upload.start", nil)
		return
	}
	if frame.Path == "" {
		s.sendError(frame.ID, "InvalidFrame", "path is required for file.upload.start", nil)
		return
	}
	result, err := invokeSandboxStreamMethod(s.sandboxID, "sandbox_stream_open", map[string]interface{}{
		"type": streamTypeOrFile(frame.StreamType),
		"path": frame.Path,
		"mode": "write",
	}, streamInvokeTimeout)
	if err != nil {
		s.sendError(frame.ID, "RuntimeError", err.Error(), nil)
		return
	}
	if err := sandboxResultError(result); err != nil {
		s.sendError(frame.ID, "RuntimeError", err.Error(), mapResult(result))
		return
	}
	streamID, _ := mapResult(result)["stream_id"].(string)
	if streamID == "" {
		s.sendError(frame.ID, "RuntimeError", "runtime did not return stream_id", mapResult(result))
		return
	}
	s.uploads[frame.ID] = &uploadState{path: frame.Path, streamID: streamID}
	s.send(streamResponseFrame{ID: frame.ID, Type: "file.upload.ready", Path: frame.Path, StreamID: streamID})
}

func (s *sandboxStreamSession) finishUpload(frame streamControlFrame) {
	state, ok := s.uploads[frame.ID]
	if !ok {
		s.sendError(frame.ID, "UnknownUpload", "upload id is not active", nil)
		return
	}
	delete(s.uploads, frame.ID)
	if _, err := s.closeRuntimeStream(state.streamID); err != nil {
		s.sendError(frame.ID, "RuntimeError", err.Error(), nil)
		return
	}
	s.send(streamResponseFrame{
		ID:       frame.ID,
		Type:     "file.upload.done",
		Path:     state.path,
		StreamID: state.streamID,
		Bytes:    state.offset,
	})
}

func (s *sandboxStreamSession) handleBinary(frame []byte) {
	opID, payload, err := parseStreamBinaryFrame(frame)
	if err != nil {
		s.sendError("", "InvalidBinaryFrame", err.Error(), nil)
		return
	}
	state, ok := s.uploads[opID]
	if !ok {
		s.sendError(opID, "UnknownUpload", "binary frame op id is not an active upload", nil)
		return
	}

	result, err := invokeSandboxStreamMethod(s.sandboxID, "sandbox_stream_send", map[string]interface{}{
		"stream_id": state.streamID,
		"data":      payload,
	}, streamInvokeTimeout)
	if err != nil {
		s.sendError(opID, "RuntimeError", err.Error(), nil)
		return
	}
	if err := sandboxResultError(result); err != nil {
		s.sendError(opID, "RuntimeError", err.Error(), mapResult(result))
		return
	}

	m := mapResult(result)
	bytesWritten := int64(len(payload))
	if v, ok := m["bytes_written"]; ok {
		bytesWritten = asStreamInt64(v)
	}
	state.offset += bytesWritten
	if v, ok := m["offset"]; ok {
		state.offset = asStreamInt64(v)
	}
	s.send(streamResponseFrame{
		ID:           opID,
		Type:         "file.upload.ack",
		Path:         state.path,
		StreamID:     state.streamID,
		Offset:       state.offset,
		BytesWritten: bytesWritten,
	})
}

func (s *sandboxStreamSession) cleanupUploads() {
	for id, state := range s.uploads {
		if state == nil || state.streamID == "" {
			continue
		}
		if _, err := s.closeRuntimeStream(state.streamID); err != nil {
			log.GetLogger().Warnf("sandbox stream cleanup upload failed sandboxID=%s id=%s streamID=%s: %v", s.sandboxID, id, state.streamID, err)
		}
		delete(s.uploads, id)
	}
}

func (s *sandboxStreamSession) download(frame streamControlFrame) {
	if frame.ID == "" {
		s.sendError("", "InvalidFrame", "id is required for file.download.start", nil)
		return
	}
	if frame.Path == "" {
		s.sendError(frame.ID, "InvalidFrame", "path is required for file.download.start", nil)
		return
	}
	chunkSize := frame.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultStreamChunk
	}
	if chunkSize > maxStreamChunk {
		chunkSize = maxStreamChunk
	}

	openResult, err := invokeSandboxStreamMethod(s.sandboxID, "sandbox_stream_open", map[string]interface{}{
		"type": streamTypeOrFile(frame.StreamType),
		"path": frame.Path,
		"mode": "read",
	}, streamInvokeTimeout)
	if err != nil {
		s.sendError(frame.ID, "RuntimeError", err.Error(), nil)
		return
	}
	if err := sandboxResultError(openResult); err != nil {
		s.sendError(frame.ID, "RuntimeError", err.Error(), mapResult(openResult))
		return
	}
	streamID, _ := mapResult(openResult)["stream_id"].(string)
	if streamID == "" {
		s.sendError(frame.ID, "RuntimeError", "runtime did not return stream_id", mapResult(openResult))
		return
	}
	defer func() { _, _ = s.closeRuntimeStream(streamID) }()

	var bytesRead int64
	for {
		result, err := invokeSandboxStreamMethod(s.sandboxID, "sandbox_stream_recv", map[string]interface{}{
			"stream_id": streamID,
			"limit":     chunkSize,
		}, streamInvokeTimeout)
		if err != nil {
			s.sendError(frame.ID, "RuntimeError", err.Error(), nil)
			return
		}
		if err := sandboxResultError(result); err != nil {
			s.sendError(frame.ID, "RuntimeError", err.Error(), mapResult(result))
			return
		}
		m := mapResult(result)
		chunk, err := streamBytes(m["data"])
		if err != nil {
			s.sendError(frame.ID, "RuntimeError", err.Error(), m)
			return
		}
		if len(chunk) > 0 {
			if err := s.sendBinary(frame.ID, chunk); err != nil {
				log.GetLogger().Warnf("sandbox stream download write failed sandboxID=%s id=%s: %v", s.sandboxID, frame.ID, err)
				return
			}
			bytesRead += int64(len(chunk))
		}
		eof, _ := m["eof"].(bool)
		if eof {
			s.send(streamResponseFrame{
				ID:        frame.ID,
				Type:      "file.download.done",
				Path:      frame.Path,
				StreamID:  streamID,
				BytesRead: bytesRead,
				EOF:       true,
			})
			return
		}
	}
}

func (s *sandboxStreamSession) closeRuntimeStream(streamID string) (interface{}, error) {
	if streamID == "" {
		return nil, fmt.Errorf("stream_id is required")
	}
	result, err := invokeSandboxStreamMethod(s.sandboxID, "sandbox_stream_close", map[string]interface{}{
		"stream_id": streamID,
	}, streamInvokeTimeout)
	if err != nil {
		return nil, err
	}
	if err := sandboxResultError(result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *sandboxStreamSession) send(frame streamResponseFrame) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	data, err := json.Marshal(frame)
	if err != nil {
		return
	}
	if err := s.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.GetLogger().Debugf("sandbox stream write text failed sandboxID=%s: %v", s.sandboxID, err)
	}
}

func (s *sandboxStreamSession) sendError(id, code, message string, details map[string]interface{}) {
	s.send(streamResponseFrame{
		ID:      id,
		Type:    "error",
		Code:    code,
		Message: message,
		Details: details,
	})
}

func (s *sandboxStreamSession) sendBinary(opID string, payload []byte) error {
	frame, err := buildStreamBinaryFrame(opID, payload)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func buildStreamBinaryFrame(opID string, payload []byte) ([]byte, error) {
	if len(opID) == 0 {
		return nil, fmt.Errorf("op id is required")
	}
	if len(opID) > 65535 {
		return nil, fmt.Errorf("op id too long")
	}
	frame := make([]byte, 4+2+len(opID)+len(payload))
	copy(frame[:4], []byte(streamBinaryMagic))
	binary.BigEndian.PutUint16(frame[4:6], uint16(len(opID)))
	copy(frame[6:6+len(opID)], []byte(opID))
	copy(frame[6+len(opID):], payload)
	return frame, nil
}

func parseStreamBinaryFrame(frame []byte) (string, []byte, error) {
	if len(frame) < 6 {
		return "", nil, fmt.Errorf("binary frame too short")
	}
	if string(frame[:4]) != streamBinaryMagic {
		return "", nil, fmt.Errorf("invalid binary frame magic")
	}
	idLen := int(binary.BigEndian.Uint16(frame[4:6]))
	if idLen == 0 {
		return "", nil, fmt.Errorf("binary frame op id is required")
	}
	if len(frame) < 6+idLen {
		return "", nil, fmt.Errorf("binary frame truncated")
	}
	return string(frame[6 : 6+idLen]), frame[6+idLen:], nil
}

func invokeSandboxStreamMethod(instanceID, method string, args map[string]interface{}, timeout int) (interface{}, error) {
	normalizedArgs, ok := normalizeJSONValue(args).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("stream args must be a map")
	}
	packedArgs, err := packKwargs(normalizedArgs)
	if err != nil {
		return nil, err
	}
	data, err := util.NewClient().InvokeInstanceByLibRtAndGet(sandboxMethodMeta(method), instanceID, packedArgs, api.InvokeOptions{
		Timeout:          timeout,
		BypassDataSystem: true,
	})
	if err != nil {
		return nil, err
	}
	return decodeYRValue(data)
}

func streamBytes(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("runtime returned unsupported stream data type %T", value)
	}
}

func asStreamInt64(value interface{}) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case uint64:
		return int64(v)
	case uint:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func sandboxResultError(result interface{}) error {
	if m := mapResult(result); m != nil {
		if errVal, ok := m["error"]; ok && errVal != nil && fmt.Sprint(errVal) != "" {
			return fmt.Errorf("%v", errVal)
		}
	}
	return nil
}

func mapResult(result interface{}) map[string]interface{} {
	if m, ok := result.(map[string]interface{}); ok {
		return m
	}
	return nil
}
