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
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/config"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins, authentication is handled via JWT
	},
}

// Session represents a WebSocket session
type Session struct {
	conn      *websocket.Conn
	tenantID  string
	clientID  string
	processor *Processor
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// HandlePosixWebSocket handles WebSocket connections for POSIX operations
func HandlePosixWebSocket(w http.ResponseWriter, r *http.Request) {
	// 1. JWT Authentication
	tenantID, err := authenticateWebSocket(r)
	if err != nil {
		log.GetLogger().Errorf("WebSocket authentication failed from %s: %v", r.RemoteAddr, err)
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	// 2. WebSocket upgrade
	var upgradeHeader http.Header
	if protos := websocket.Subprotocols(r); len(protos) > 0 {
		upgradeHeader = http.Header{"Sec-WebSocket-Protocol": []string{protos[0]}}
	}
	conn, err := upgrader.Upgrade(w, r, upgradeHeader)
	if err != nil {
		log.GetLogger().Errorf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// 3. Create session
	ctx, cancel := context.WithCancel(context.Background())
	session := &Session{
		conn:      conn,
		tenantID:  tenantID,
		processor: NewProcessor(),
		ctx:       ctx,
		cancel:    cancel,
	}

	log.GetLogger().Infof("POSIX WebSocket connected, tenant: %s", tenantID)

	// 4. Message loop
	session.run()
}

// authenticateWebSocket performs JWT authentication for WebSocket connection
func authenticateWebSocket(r *http.Request) (string, error) {
	// Skip auth if disabled
	if !config.GetConfig().IamConfig.EnableFuncTokenAuth {
		if r.URL.Query().Get("tenant_id") != "" {
			return r.URL.Query().Get("tenant_id"), nil
		}
		return "default", nil
	}

	// Get token from various sources
	token := r.Header.Get("X-Auth")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		if cookie, err := r.Cookie("iam_token"); err == nil {
			token = cookie.Value
		}
	}
	// Fall back to Sec-WebSocket-Protocol (browser WebSocket subprotocol trick)
	if token == "" {
		for _, proto := range websocket.Subprotocols(r) {
			if proto != "" {
				token = proto
				break
			}
		}
	}

	if token == "" {
		return "", ErrNoToken
	}

	// Parse and validate JWT
	parsedJWT, err := jwtauth.ParseJWT(token)
	if err != nil {
		return "", err
	}

	// Validate with IAM server
	if err := jwtauth.ValidateWithIamServer(token, r.Header.Get("X-Trace-ID")); err != nil {
		return "", err
	}

	tenantID := parsedJWT.Payload.Sub
	if tenantID == "" {
		tenantID = "default"
	}

	return tenantID, nil
}

// pongWait is the deadline for reading the next pong/message from the client.
const pongWait = 60 * time.Second

// run starts the message processing loop
func (s *Session) run() {
	// Reset read deadline whenever a pong (or ping response) arrives,
	// so long-running requests don't cause the connection to time out.
	s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	// WriteControl is safe for concurrent use with WriteMessage (gorilla docs),
	// so we must NOT take s.mu here — it would block ReadMessage while response
	// goroutines hold s.mu, starving the ping/pong flow and causing silent
	// read-deadline expiry under high concurrency.
	s.conn.SetPingHandler(func(appData string) error {
		s.conn.SetReadDeadline(time.Now().Add(pongWait))
		return s.conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			messageType, message, err := s.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.GetLogger().Warnf("POSIX WebSocket closed unexpectedly, tenant=%s: %v", s.tenantID, err)
				} else {
					log.GetLogger().Infof("POSIX WebSocket closed, tenant=%s: %v", s.tenantID, err)
				}
				return
			}

			// Reset read deadline after successful read
			s.conn.SetReadDeadline(time.Now().Add(pongWait))

			// Handle message asynchronously so read loop is not blocked
			// by long-running create/invoke operations
			go s.handleMessage(messageType, message)
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (s *Session) handleMessage(messageType int, message []byte) {
	if messageType == websocket.BinaryMessage {
		s.handleBinaryMessage(message)
		return
	}

	// JSON text message path (backward compatible)
	// Try to parse as control message first
	var ctrlMsg ControlMessage
	if err := json.Unmarshal(message, &ctrlMsg); err == nil {
		if ctrlMsg.Type == MessageTypePing {
			s.sendPong()
			return
		}
	}

	// Parse as request message
	reqMsg, err := ParseRequestMessage(message)
	if err != nil {
		log.GetLogger().Debugf("Failed to parse request message: %v", err)
		s.sendErrorResponse("", ErrInvalidMessage)
		return
	}

	// Process the request
	var response *ResponseMessage
	switch reqMsg.Operation {
	case OperationCreate:
		response = s.processCreate(reqMsg)
	case OperationInvoke:
		response = s.processInvoke(reqMsg)
	default:
		response = NewErrorResponse(reqMsg.ID, ErrInvalidOperation.Code, ErrInvalidOperation.Message)
	}

	// Send response
	s.sendResponse(response)
}

// handleBinaryMessage processes binary frame messages
func (s *Session) handleBinaryMessage(data []byte) {
	binReq, err := ParseBinaryRequest(data)
	if err != nil {
		log.GetLogger().Errorf("Failed to parse binary request: %v", err)
		return
	}

	headers := map[string]string{
		"X-Tenant-Id": s.tenantID,
	}

	var resp []byte
	var procErr error
	switch binReq.Operation {
	case BinOpCreate:
		resp, procErr = s.processor.ProcessCreateRaw(s.ctx, binReq.Payload, headers)
	case BinOpInvoke:
		resp, procErr = s.processor.ProcessInvokeRaw(s.ctx, binReq.Payload, headers)
	default:
		log.GetLogger().Errorf("WS unknown op: %d, id=%s", binReq.Operation, binReq.ID)
		s.sendBinaryError(binReq.ID, ErrInvalidOperation.Code, ErrInvalidOperation.Message)
		return
	}

	if procErr != nil {
		log.GetLogger().Errorf("WS request failed, id=%s: %v", binReq.ID, procErr)
		s.sendBinaryError(binReq.ID, ErrProcessFailed.Code, procErr.Error())
		return
	}
	s.sendBinaryResponse(binReq.ID, resp)
}

// sendBinaryResponse sends a success binary response frame
func (s *Session) sendBinaryResponse(reqID string, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	frame := BuildBinaryResponse(reqID, BinStatusOK, payload)
	if err := s.conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		log.GetLogger().Debugf("Failed to send binary response: %v", err)
	}
}

// sendBinaryError sends an error binary response frame
func (s *Session) sendBinaryError(reqID string, code int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	errPayload := BuildBinaryErrorPayload(code, message)
	frame := BuildBinaryResponse(reqID, BinStatusError, errPayload)
	if err := s.conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		log.GetLogger().Debugf("Failed to send binary error response: %v", err)
	}
}

// processCreate handles create operation
func (s *Session) processCreate(req *RequestMessage) *ResponseMessage {
	// Add tenant ID to headers
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers["X-Tenant-Id"] = s.tenantID

	payload, err := s.processor.ProcessCreate(s.ctx, req.Payload, req.Headers)
	if err != nil {
		return NewErrorResponse(req.ID, ErrProcessFailed.Code, err.Error())
	}
	return NewSuccessResponse(req.ID, payload)
}

// processInvoke handles invoke operation
func (s *Session) processInvoke(req *RequestMessage) *ResponseMessage {
	// Add tenant ID to headers
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers["X-Tenant-Id"] = s.tenantID

	payload, err := s.processor.ProcessInvoke(s.ctx, req.Payload, req.Headers)
	if err != nil {
		return NewErrorResponse(req.ID, ErrProcessFailed.Code, err.Error())
	}
	return NewSuccessResponse(req.ID, payload)
}

// sendResponse sends a response message
func (s *Session) sendResponse(resp *ResponseMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := resp.ToJSON()
	if err != nil {
		log.GetLogger().Debugf("Failed to marshal response: %v", err)
		return
	}

	if err := s.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.GetLogger().Debugf("Failed to send response: %v", err)
	}
}

// sendErrorResponse sends an error response
func (s *Session) sendErrorResponse(id string, err *ErrorInfo) {
	s.sendResponse(NewErrorResponse(id, err.Code, err.Message))
}

// sendPong sends a pong control message
func (s *Session) sendPong() {
	s.mu.Lock()
	defer s.mu.Unlock()

	pong := NewPongMessage(time.Now().Unix())
	data, err := pong.ToJSON()
	if err != nil {
		return
	}

	s.conn.WriteMessage(websocket.TextMessage, data)
}

// Error definitions
var (
	ErrNoToken          = &ErrorInfo{Code: 1001, Message: "no authentication token provided"}
	ErrInvalidToken     = &ErrorInfo{Code: 1002, Message: "invalid authentication token"}
	ErrInvalidMessage   = &ErrorInfo{Code: 1003, Message: "invalid message format"}
	ErrInvalidOperation = &ErrorInfo{Code: 1004, Message: "invalid operation type"}
	ErrProcessFailed    = &ErrorInfo{Code: 1005, Message: "operation processing failed"}
)
