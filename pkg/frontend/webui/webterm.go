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

package webui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/grpc/pb/exec_service"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/config"
)

//go:embed static/*
var StaticFiles embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var (
	defaultCommand []string = []string{"/bin/bash"}
	defaultTTY     bool     = true
	defaultRows    int32    = 24
	defaultCols    int32    = 80
)

// grpcConnPool maintains shared gRPC connections keyed by proxy address.
// All sessions to the same proxy reuse a single HTTP/2 TCP connection so that
// closing one ExecStream RPC only tears down its HTTP/2 stream, NOT the
// underlying TCP connection.  A per-session NewClient() + Close() sends a
// FIN/GOAWAY to the server the moment that session ends, which gRPC-core C++
// propagates to the adjacent connection – causing cascade disconnects across
// all open sessions on the same proxy.
var (
	grpcPool   = map[string]*pooledConn{}
	grpcPoolMu sync.Mutex
)

type pooledConn struct {
	conn   *grpc.ClientConn
	refCnt int
}

func acquireGrpcConn(addr string) (*grpc.ClientConn, error) {
	grpcPoolMu.Lock()
	defer grpcPoolMu.Unlock()
	if pc, ok := grpcPool[addr]; ok {
		pc.refCnt++
		return pc.conn, nil
	}
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    1 * time.Hour,
			Timeout: 10 * time.Second,
		}),
	)
	if err != nil {
		return nil, err
	}
	grpcPool[addr] = &pooledConn{conn: conn, refCnt: 1}
	log.GetLogger().Infof("[pool] new gRPC connection addr=%s", addr)
	return conn, nil
}

func releaseGrpcConn(addr string) {
	grpcPoolMu.Lock()
	defer grpcPoolMu.Unlock()
	pc, ok := grpcPool[addr]
	if !ok {
		log.GetLogger().Infof("[pool] releaseGrpcConn MISS addr=%s (already deleted)", addr)
		return
	}
	pc.refCnt--
	if pc.refCnt <= 0 {
		log.GetLogger().Infof("[pool] releaseGrpcConn CLOSE conn addr=%s", addr)
		pc.conn.Close()
		delete(grpcPool, addr)
	}
}

type wsSession struct {
	ws         *websocket.Conn
	grpcStream exec_service.ExecService_ExecStreamClient
	sessionID  string
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
}

// InstanceStatus defines instance status structure
type InstanceStatus struct {
	Code     int    `json:"code"`     // Status code
	ExitCode int    `json:"exitCode"` // Exit code
	Msg      string `json:"msg"`      // Status message
	Type     int    `json:"type"`     // Type
	ErrCode  int    `json:"errCode"`  // Error code
}

// Resources defines resource configuration
type Resources struct {
	CPU    string `json:"cpu"`    // CPU quota, e.g. "2000m"
	Memory string `json:"memory"` // Memory quota, e.g. "4Gi"
}

// InstanceInfo defines instance information structure (corresponding to instance returned by master API)
type InstanceInfo struct {
	InstanceID       string         `json:"instanceID"`       // Instance ID
	TenantID         string         `json:"tenantID"`         // Tenant ID
	ContainerID      string         `json:"containerID"`      // Container ID
	ProxyGrpcAddress string         `json:"proxyGrpcAddress"` // Proxy gRPC address
	FunctionProxyID  string         `json:"functionProxyID"`  // Function Proxy ID
	Function         string         `json:"function"`         // Function name
	RuntimeAddress   string         `json:"runtimeAddress"`   // Runtime address
	RuntimeID        string         `json:"runtimeID"`        // Runtime ID
	InstanceStatus   InstanceStatus `json:"instanceStatus"`   // Instance status
	Resources        Resources      `json:"resources"`        // Resource configuration
	StartTime        string         `json:"startTime"`        // Start time
	RequestID        string         `json:"requestID"`        // Request ID
	ParentID         string         `json:"parentID"`         // Parent ID
	JobID            string         `json:"jobID"`            // Job ID
	NodeIP           string         `json:"nodeIP"`           // Node IP
	NodePort         string         `json:"nodePort"`         // Node port
}

// InstanceListResponse defines instance list response structure (corresponding to master API response)
type InstanceListResponse struct {
	Instances []InstanceInfo `json:"instances"` // Instance list
	Count     int            `json:"count"`     // Instance count
	TenantID  string         `json:"tenantID"`  // Tenant ID
}

// queryMaster is a generic function to query the master
// apiPath: API path, e.g. "/api/v1/containers" or "/api/v1/container/node"
// queryParams: Query parameter map, e.g. map[string]string{"container": "xxx"}
// result: Pointer to the structure to receive the response
func queryMaster(apiPath string, queryParams map[string]string, result interface{}) error {
	// Get master address
	masterAddr := util.NewClient().GetActiveMasterAddr()
	if masterAddr == "" {
		return fmt.Errorf("failed to get master address")
	}

	// Build query URL
	var queryURL string
	baseURL := fmt.Sprintf("http://%s%s", masterAddr, apiPath)
	if len(queryParams) > 0 {
		params := url.Values{}
		for k, v := range queryParams {
			params.Add(k, v)
		}
		queryURL = baseURL + "?" + params.Encode()
	} else {
		queryURL = baseURL
	}

	// Create HTTP request
	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// TODO: Add request headers as needed
	// req.Header.Set("Authorization", "Bearer <token>")
	req.Header.Set("Content-Type", "application/json")

	// Make HTTP request with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to query master: %w", err)
	}
	defer resp.Body.Close()

	// Check response status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("master returned error status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

func getExecAddr(instance, tenantID string) (InstanceInfo, error) {
	if instance == "" {
		return InstanceInfo{}, fmt.Errorf("instance ID cannot be empty")
	}

	if tenantID == "" {
		tenantID = "default"
	}

	// Query all instances and find the matching one
	apiPath := "/instance-manager/query-tenant-instances"
	queryParams := map[string]string{
		"tenant_id": tenantID,
	}

	// Call generic query function
	var response InstanceListResponse
	if err := queryMaster(apiPath, queryParams, &response); err != nil {
		return InstanceInfo{}, fmt.Errorf("failed to query instances: %w", err)
	}

	// Find matching instance (supports matching by instanceID)
	for _, inst := range response.Instances {
		if inst.InstanceID == instance {
			if inst.ProxyGrpcAddress == "" {
				return InstanceInfo{}, fmt.Errorf("proxy gRPC address is empty for instance %s", instance)
			}
			log.GetLogger().Infof("Instance %s found on node: %s (proxy: %s)",
				instance, inst.NodeIP, inst.ProxyGrpcAddress)
			return inst, nil
		}
	}

	return InstanceInfo{}, fmt.Errorf("instance %s not found", instance)
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}
	// Authenticate JWT token from query parameter, header, or WebSocket subprotocol.
	// Note: browsers cannot set custom HTTP headers on WebSocket connections.
	// Passing the token as a Sec-WebSocket-Protocol subprotocol is the standard workaround.
	if config.GetConfig().IamConfig.EnableFuncTokenAuth {
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
			log.GetLogger().Errorf("WebSocket authentication failed: no token provided")
			http.Error(w, "authentication failed: no token provided", http.StatusUnauthorized)
			return
		}
		// Parse JWT to validate
		parsedJWT, err := jwtauth.ParseJWT(token)
		if err != nil {
			log.GetLogger().Errorf("WebSocket JWT parsing failed: %v", err)
			http.Error(w, "authentication failed: invalid token", http.StatusUnauthorized)
			return
		}

		// Validate with IAM server
		if err := jwtauth.ValidateWithIamServer(token, r.Header.Get("X-Trace-ID")); err != nil {
			log.GetLogger().Errorf("WebSocket IAM server validation failed: %v", err)
			http.Error(w, "authentication failed: IAM server validation failed", http.StatusUnauthorized)
			return
		}

		if parsedJWT.Payload.Sub != "" {
			tenantID = parsedJWT.Payload.Sub
		}

		log.GetLogger().Infof("WebSocket JWT authentication passed, role: %s, tenant: %s",
			parsedJWT.Payload.Role, tenantID)
	}

	// Log client certificate info if TLS is enabled (verification already done at TLS handshake)
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		clientCert := r.TLS.PeerCertificates[0]
		log.GetLogger().Infof("Client connected with certificate: Subject=%s, Issuer=%s",
			clientCert.Subject.String(), clientCert.Issuer.String())
	}

	// Echo back the subprotocol so the browser accepts the upgrade.
	// If token was sent via Sec-WebSocket-Protocol we must mirror it in the response.
	var upgradeHeader http.Header
	if protos := websocket.Subprotocols(r); len(protos) > 0 {
		upgradeHeader = http.Header{"Sec-WebSocket-Protocol": []string{protos[0]}}
	}
	conn, err := upgrader.Upgrade(w, r, upgradeHeader)
	if err != nil {
		log.GetLogger().Infof("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	sessionID := uuid.New().String()
	log.GetLogger().Infof("WebSocket client connected, session: %s", sessionID)

	// Read configuration from URL parameters
	query := r.URL.Query()
	instance := query.Get("instance")

	cmdStr := query.Get("command")
	command := defaultCommand
	if cmdStr != "" {
		command = []string{cmdStr}
	}

	tty := defaultTTY
	if query.Get("tty") == "false" {
		tty = false
	}

	rows := defaultRows
	if r := query.Get("rows"); r != "" {
		if val, err := fmt.Sscanf(r, "%d", &rows); err == nil && val == 1 {
			// rows updated
		}
	}

	cols := defaultCols
	if c := query.Get("cols"); c != "" {
		if val, err := fmt.Sscanf(c, "%d", &cols); err == nil && val == 1 {
			// cols updated
		}
	}

	// Connect to executor backend
	info, err := getExecAddr(instance, tenantID)
	if err != nil {
		log.GetLogger().Infof("Failed to get executor address: %v", err)
		return
	}
	grpcConn, err := acquireGrpcConn(info.ProxyGrpcAddress)
	if err != nil {
		log.GetLogger().Infof("Failed to connect to executor: %v", err)
		return
	}
	defer releaseGrpcConn(info.ProxyGrpcAddress)

	client := exec_service.NewExecServiceClient(grpcConn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.ExecStream(ctx)
	if err != nil {
		log.GetLogger().Infof("Failed to create ExecStream: %v", err)
		return
	}

	session := &wsSession{
		ws:         conn,
		grpcStream: stream,
		sessionID:  sessionID,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Wait for initial frontend terminal size before creating backend exec session.
	// Browser side sends: RESIZE:cols:rows
	type pendingInput struct {
		messageType int
		data        []byte
	}
	pendingInputs := make([]pendingInput, 0)
	if tty {
		readTimeout := 1 * time.Second
		if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			log.GetLogger().Infof("Session %s: failed to set read deadline: %v", sessionID, err)
		}

		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.GetLogger().Infof("Session %s: initial RESIZE not received in %s, fallback to default size=%dx%d",
						sessionID, readTimeout, cols, rows)
					if clearErr := conn.SetReadDeadline(time.Time{}); clearErr != nil {
						log.GetLogger().Infof("Session %s: failed to clear read deadline after initial timeout: %v", sessionID, clearErr)
						return
					}
					break
				}
				log.GetLogger().Infof("Session %s: failed waiting initial terminal size: %v", sessionID, err)
				return
			}

			if messageType == websocket.TextMessage && len(message) > 7 && string(message[:7]) == "RESIZE:" {
				var newCols, newRows int32
				if n, _ := fmt.Sscanf(string(message), "RESIZE:%d:%d", &newCols, &newRows); n == 2 && newCols > 0 && newRows > 0 {
					cols = newCols
					rows = newRows
					log.GetLogger().Infof("Session %s: received initial terminal size=%dx%d", sessionID, cols, rows)
					break
				}
			}

			// Buffer any early input and replay after session starts.
			if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
				buf := make([]byte, len(message))
				copy(buf, message)
				pendingInputs = append(pendingInputs, pendingInput{messageType: messageType, data: buf})
			}
		}

		if err := conn.SetReadDeadline(time.Time{}); err != nil {
			log.GetLogger().Infof("Session %s: failed to clear read deadline: %v", sessionID, err)
			return
		}
	}

	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		log.GetLogger().Infof("Session %s: failed to ensure read deadline is cleared: %v", sessionID, err)
		return
	}

	// Send start request
	log.GetLogger().Infof("Starting: instance=%s, command=%v, tty=%v, size=%dx%d",
		instance, command, tty, cols, rows)
	err = stream.Send(&exec_service.ExecMessage{
		SessionId: sessionID,
		Payload: &exec_service.ExecMessage_StartRequest{
			StartRequest: &exec_service.ExecStartRequest{
				ContainerId: info.ContainerID,
				Command:     command,
				Tty:         tty,
				Rows:        rows,
				Cols:        cols,
				InstanceId:  info.InstanceID,
			},
		},
	})
	if err != nil {
		log.GetLogger().Infof("Failed to send start request: %v", err)
		return
	}

	for _, msg := range pendingInputs {
		err := stream.Send(&exec_service.ExecMessage{
			SessionId: sessionID,
			Payload: &exec_service.ExecMessage_InputData{
				InputData: &exec_service.ExecInputData{
					Data: msg.data,
				},
			},
		})
		if err != nil {
			log.GetLogger().Infof("Session %s: failed to replay early input: %v", sessionID, err)
			return
		}
	}

	done := make(chan struct{})

	// Read output from gRPC and send to WebSocket
	go func() {
		defer func() {
			select {
			case <-done:
			default:
				close(done)
			}
			// When gRPC stream closes, close WebSocket connection
			conn.Close()
		}()

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				log.GetLogger().Infof("Session %s: gRPC stream closed", sessionID)
				return
			}
			if err != nil {
				log.GetLogger().Infof("Session %s: gRPC recv error: %v", sessionID, err)
				return
			}

			switch payload := msg.Payload.(type) {
			case *exec_service.ExecMessage_OutputData:
				session.mu.Lock()
				err := conn.WriteMessage(websocket.BinaryMessage, payload.OutputData.Data)
				session.mu.Unlock()
				if err != nil {
					log.GetLogger().Infof("WebSocket write error: %v", err)
					return
				}

			case *exec_service.ExecMessage_Status:
				log.GetLogger().Infof("Session %s: status=%v, exit_code=%d, error=%s",
					sessionID, payload.Status.Status, payload.Status.ExitCode, payload.Status.ErrorMessage)

				if payload.Status.Status == exec_service.ExecStatusResponse_EXITED ||
					payload.Status.Status == exec_service.ExecStatusResponse_ERROR {
					// Notify WebSocket client that process has exited
					session.mu.Lock()
					conn.WriteMessage(websocket.TextMessage, []byte("\r\n[Process exited]\r\n"))
					conn.WriteControl(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
						time.Now().Add(time.Second))
					session.mu.Unlock()
					return
				}
			}
		}
	}()

	// Read input from WebSocket and send to gRPC
	go func() {
		defer func() {
			select {
			case <-done:
			default:
				close(done)
			}
			// When WebSocket disconnects, cancel gRPC context to notify backend
			cancel()
		}()

		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.GetLogger().Infof("WebSocket read error: %v", err)
				}
				return
			}

			switch messageType {
			case websocket.TextMessage:
				// Check if it's a resize message (format: "RESIZE:cols:rows")
				if len(message) > 7 && string(message[:7]) == "RESIZE:" {
					var newCols, newRows int32
					if n, _ := fmt.Sscanf(string(message), "RESIZE:%d:%d", &newCols, &newRows); n == 2 {
						err := stream.Send(&exec_service.ExecMessage{
							SessionId: sessionID,
							Payload: &exec_service.ExecMessage_Resize{
								Resize: &exec_service.ExecResizeRequest{
									Rows: newRows,
									Cols: newCols,
								},
							},
						})
						if err != nil {
							log.GetLogger().Infof("gRPC resize error: %v", err)
						}
						break
					}
				}
				fallthrough
			case websocket.BinaryMessage:
				err := stream.Send(&exec_service.ExecMessage{
					SessionId: sessionID,
					Payload: &exec_service.ExecMessage_InputData{
						InputData: &exec_service.ExecInputData{
							Data: message,
						},
					},
				})
				if err != nil {
					log.GetLogger().Infof("gRPC send error: %v", err)
					return
				}
			}
		}
	}()

	<-done
	log.GetLogger().Infof("Session %s disconnected", sessionID)
}

// HandleInstances returns instance list, queried from master
func HandleInstances(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get tenant_id from request parameters, use default if not provided
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}

	// Call master's instance management API
	apiPath := "/instance-manager/query-tenant-instances"
	queryParams := map[string]string{
		"tenant_id": tenantID,
	}

	// Call generic query function
	var response InstanceListResponse
	if err := queryMaster(apiPath, queryParams, &response); err != nil {
		log.GetLogger().Infof("Failed to query instances from master: %v", err)
		// Return empty list on query failure instead of error, so frontend can continue
		response.Instances = []InstanceInfo{}
	}

	// Convert to frontend expected format (simplified instance info)
	instances := make([]map[string]interface{}, 0, len(response.Instances))
	for _, inst := range response.Instances {
		errorDetail := fmt.Sprintf("msg=%s; code=%d; exitCode=%d; errCode=%d",
			inst.InstanceStatus.Msg, inst.InstanceStatus.Code, inst.InstanceStatus.ExitCode, inst.InstanceStatus.ErrCode)
		statusText := instanceStatusText(inst.InstanceStatus.Code)
		if statusText == "unknown" {
			statusText = inst.InstanceStatus.Msg
		}
		instance := map[string]interface{}{
			"id":       inst.InstanceID,
			"function": inst.Function,
			"status":   statusText,
			"error":    errorDetail,
		}
		instances = append(instances, instance)
	}

	// Return instance list
	if err := json.NewEncoder(w).Encode(instances); err != nil {
		log.GetLogger().Infof("Error encoding instances: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func instanceStatusText(code int) string {
	switch constant.InstanceStatus(code) {
	case constant.KernelInstanceStatusExited:
		return "exited"
	case constant.KernelInstanceStatusNew:
		return "new"
	case constant.KernelInstanceStatusScheduling:
		return "scheduling"
	case constant.KernelInstanceStatusCreating:
		return "creating"
	case constant.KernelInstanceStatusRunning:
		return "running"
	case constant.KernelInstanceStatusFailed:
		return "failed"
	case constant.KernelInstanceStatusExiting:
		return "exiting"
	case constant.KernelInstanceStatusFatal:
		return "fatal"
	case constant.KernelInstanceStatusScheduleFailed:
		return "schedule_failed"
	case constant.KernelInstanceStatusEvicting:
		return "evicting"
	case constant.KernelInstanceStatusEvicted:
		return "evicted"
	case constant.KernelInstanceStatusSubHealth:
		return "sub_health"
	default:
		return "unknown"
	}
}

func HandleIndex(w http.ResponseWriter, r *http.Request) {
	// Get path prefix from X-Forwarded-Prefix header (set by traefik/reverse proxy)
	// or from environment variable, default to empty string
	pathPrefix := r.Header.Get("X-Forwarded-Prefix")
	if pathPrefix == "" {
		// Fallback to environment variable if header is not set
		// Set PATH_PREFIX environment variable in deployment config if needed
		// For example: PATH_PREFIX=/frontend
		// pathPrefix = os.Getenv("PATH_PREFIX")
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Remote Exec Terminal</title>
    <meta charset="UTF-8">
    <!--
        This page uses xterm.js - Copyright (c) 2017-2022, The xterm.js authors
        Licensed under the MIT License - https://github.com/xtermjs/xterm.js
    -->
    <link rel="stylesheet" href="%s/terminal/static/xterm.css" />
    <style>
        body {
            margin: 0;
            padding: 0;
            background: #1e1e1e;
            font-family: 'Courier New', monospace;
            display: flex;
            flex-direction: column;
            height: 100vh;
        }
        #header {
            background: #2d2d30;
            color: #ccc;
            padding: 10px 20px;
            border-bottom: 1px solid #3e3e42;
            display: flex;
            justify-content: flex-start;
            align-items: center;
            gap: 12px;
        }
        #toggle-sidebar-btn {
            background: transparent;
            color: #d4d4d4;
            border: 1px solid #555;
            padding: 4px 10px;
            border-radius: 3px;
            cursor: pointer;
            font-size: 12px;
            outline: none;
            transition: all 0.2s;
        }
        #toggle-sidebar-btn:hover {
            background: #3c3c3c;
            border-color: #777;
        }
        .home-link {
            color: #ccc;
            text-decoration: none;
            display: inline-flex;
            align-items: center;
            justify-content: center;
            min-width: 28px;
            height: 28px;
            border: 1px solid #555;
            border-radius: 3px;
            padding: 0 8px;
            font-size: 13px;
            transition: all 0.2s;
            opacity: 0.9;
            box-sizing: border-box;
        }
        .home-link:hover {
            background: #3c3c3c;
            border-color: #777;
            opacity: 1;
        }
        #status {
            display: flex;
            align-items: center;
            gap: 10px;
            margin-left: auto;
        }
        .status-indicator {
            width: 8px;
            height: 8px;
            border-radius: 50%%;
            background: #666;
        }
        .status-indicator.connected {
            background: #4caf50;
            box-shadow: 0 0 5px #4caf50;
        }
        .status-indicator.disconnected {
            background: #f44336;
        }
        #main-container {
            display: flex;
            flex: 1;
            overflow: hidden;
        }
        #main-container.sidebar-hidden #sidebar {
            display: none;
        }
        #sidebar {
            width: 280px;
            background: #252526;
            border-right: 1px solid #3e3e42;
            display: flex;
            flex-direction: column;
        }
        #sidebar-header {
            padding: 12px 16px;
            background: #2d2d30;
            border-bottom: 1px solid #3e3e42;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        #sidebar-header h2 {
            margin: 0;
            font-size: 13px;
            font-weight: normal;
            color: #ccc;
        }
        #refresh-btn {
            background: transparent;
            color: #d4d4d4;
            border: none;
            padding: 4px 8px;
            cursor: pointer;
            font-size: 14px;
            outline: none;
            opacity: 0.7;
            transition: opacity 0.2s;
        }
        #refresh-btn:hover {
            opacity: 1;
        }
        #instance-list {
            flex: 1;
            overflow-y: auto;
            padding: 4px 0;
        }
        .instance-item {
            padding: 10px 16px;
            cursor: pointer;
            color: #ccc;
            font-size: 13px;
            border-left: 3px solid transparent;
            transition: background 0.2s;
            display: flex;
            flex-direction: column;
            gap: 4px;
        }
        .instance-item:hover {
            background: #2a2d2e;
        }
        .instance-item.active {
            background: #37373d;
            border-left-color: #007acc;
        }
        .instance-item .instance-id {
            font-weight: 500;
            color: #d4d4d4;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
        .instance-item .instance-head {
            display: flex;
            align-items: center;
            justify-content: space-between;
            gap: 8px;
        }
        .instance-item .instance-delete-btn {
            background: transparent;
            color: #bbb;
            border: 1px solid #555;
            border-radius: 3px;
            width: 22px;
            height: 22px;
            line-height: 18px;
            padding: 0;
            cursor: pointer;
            opacity: 0.8;
            flex: 0 0 auto;
        }
        .instance-item .instance-delete-btn:hover {
            background: #3c3c3c;
            border-color: #777;
            opacity: 1;
        }
        .instance-item .instance-status {
            font-size: 11px;
        }
        .instance-item .instance-status.with-error {
            text-decoration: underline dotted #777;
            cursor: help;
        }
        .instance-item .instance-status.running {
            color: #4caf50;
        }
        .instance-item .instance-status.stopped {
            color: #f44336;
        }
        #pagination {
            padding: 8px 16px;
            background: #2d2d30;
            border-top: 1px solid #3e3e42;
            display: flex;
            justify-content: space-between;
            align-items: center;
            font-size: 12px;
            color: #888;
        }
        #pagination .page-info {
            flex: 1;
        }
        #pagination .page-controls {
            display: flex;
            gap: 5px;
        }
        #pagination button {
            background: transparent;
            color: #d4d4d4;
            border: 1px solid #555;
            padding: 4px 8px;
            border-radius: 3px;
            cursor: pointer;
            font-size: 12px;
            outline: none;
            transition: all 0.2s;
        }
        #pagination button:hover:not(:disabled) {
            background: #4c4c4c;
            border-color: #007acc;
        }
        #pagination button:disabled {
            opacity: 0.3;
            cursor: not-allowed;
        }
        #sidebar-footer {
            padding: 10px 16px;
            background: #2d2d30;
            border-top: 1px solid #3e3e42;
        }
        #add-instance-btn {
            width: 100%%;
            background: #3c3c3c;
            color: #d4d4d4;
            border: 1px solid #555;
            padding: 8px;
            border-radius: 3px;
            cursor: pointer;
            font-size: 13px;
            outline: none;
            transition: background 0.2s;
        }
        #add-instance-btn:hover {
            background: #4c4c4c;
        }
        #terminal-container {
            flex: 1;
            padding: 10px;
            overflow: hidden;
        }
        #terminal {
            height: 100%%;
        }
        #footer {
            background: #2d2d30;
            color: #888;
            padding: 5px 20px;
            border-top: 1px solid #3e3e42;
            font-size: 12px;
            text-align: center;
        }
        /* Custom dialog styles */
        #custom-dialog-overlay {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%%;
            height: 100%%;
            background: rgba(0, 0, 0, 0.7);
            z-index: 9999;
            justify-content: center;
            align-items: center;
        }
        #custom-dialog {
            background: #2d2d30;
            border: 1px solid #3e3e42;
            border-radius: 5px;
            padding: 20px;
            min-width: 400px;
            box-shadow: 0 4px 20px rgba(0, 0, 0, 0.5);
        }
        #custom-dialog h2 {
            margin: 0 0 20px 0;
            color: #d4d4d4;
            font-size: 18px;
            font-weight: normal;
        }
        #custom-dialog .form-group {
            margin-bottom: 15px;
        }
        #custom-dialog label {
            display: block;
            margin-bottom: 5px;
            color: #d4d4d4;
            font-size: 13px;
        }
        #custom-dialog input {
            width: 100%%;
            padding: 8px;
            background: #3c3c3c;
            border: 1px solid #555;
            border-radius: 3px;
            color: #d4d4d4;
            font-size: 13px;
            box-sizing: border-box;
        }
        #custom-dialog input:focus {
            outline: none;
            border-color: #007acc;
        }
        #custom-dialog .button-group {
            display: flex;
            justify-content: flex-end;
            gap: 10px;
            margin-top: 20px;
        }
        #custom-dialog button {
            padding: 8px 16px;
            border: none;
            border-radius: 3px;
            cursor: pointer;
            font-size: 13px;
        }
        #custom-dialog .btn-primary {
            background: #007acc;
            color: white;
        }
        #custom-dialog .btn-primary:hover {
            background: #005a9e;
        }
        #custom-dialog .btn-secondary {
            background: #3c3c3c;
            color: #d4d4d4;
        }
        #custom-dialog .btn-secondary:hover {
            background: #4c4c4c;
        }
        #custom-dialog .btn-create {
            background: #28a745;
            color: white;
        }
        #custom-dialog .btn-create:hover {
            background: #218838;
        }
        #custom-dialog .btn-create:disabled {
            background: #6c757d;
            cursor: not-allowed;
            opacity: 0.6;
        }
        #custom-dialog .tab-container {
            display: flex;
            gap: 10px;
            margin-bottom: 20px;
            border-bottom: 1px solid #3e3e42;
        }
        #custom-dialog .tab {
            padding: 10px 20px;
            background: transparent;
            border: none;
            color: #888;
            cursor: pointer;
            font-size: 14px;
            border-bottom: 2px solid transparent;
            transition: all 0.2s;
        }
        #custom-dialog .tab:hover {
            color: #d4d4d4;
        }
        #custom-dialog .tab.active {
            color: #007acc;
            border-bottom-color: #007acc;
        }
        #custom-dialog .tab-content {
            display: none;
        }
        #custom-dialog .tab-content.active {
            display: block;
        }
    </style>
</head>
<body>
    <div id="header">
        <button id="toggle-sidebar-btn" title="Show instance list" aria-label="Show instance list">☰</button>
        <a href="%s/" class="home-link" title="Home" aria-label="Home">⌂</a>
        <div id="status">
            <span id="status-text">Connecting...</span>
            <div class="status-indicator" id="status-indicator"></div>
        </div>
    </div>
    <div id="main-container" class="sidebar-hidden">
        <div id="sidebar">
            <div id="sidebar-header">
                <h2>Instance List</h2>
                <button id="refresh-btn" title="Refresh instance list">🔄</button>
            </div>
            <div id="instance-list">
                <div style="padding: 20px; text-align: center; color: #888; font-size: 12px;">
                    Loading...
                </div>
            </div>
            <div id="pagination">
                <div class="page-info">
                    <span id="page-info-text">-</span>
                </div>
                <div class="page-controls">
                    <button id="first-page-btn" title="First page">«</button>
                    <button id="prev-page-btn" title="Previous page">‹</button>
                    <button id="next-page-btn" title="Next page">›</button>
                    <button id="last-page-btn" title="Last page">»</button>
                </div>
            </div>
            <div id="sidebar-footer">
                <button id="add-instance-btn">✏️ Enter Instance ID</button>
            </div>
        </div>
        <div id="terminal-container">
            <div id="terminal"></div>
        </div>
    </div>
    <div id="footer">
        Press Ctrl+C to interrupt | Connection: <span id="ws-url"></span>
    </div>

    <!-- Custom input dialog -->
    <div id="custom-dialog-overlay">
        <div id="custom-dialog">
            <h2>🖥️ Connection Config</h2>

            <!-- Tabs -->
            <div class="tab-container">
                <button class="tab active" onclick="switchTab('connect')">Connect Instance</button>
                <button class="tab" onclick="switchTab('create')">Create Sandbox</button>
            </div>

            <!-- Connect instance tab content -->
            <div id="connect-tab" class="tab-content active">
                <div class="form-group">
                    <label for="dialog-instance">Instance Name or ID *</label>
                    <input type="text" id="dialog-instance" placeholder="Enter instance name or ID">
                </div>
                <div class="form-group">
                    <label for="dialog-tenant">Tenant ID</label>
                    <input type="text" id="dialog-tenant" value="default" placeholder="Defaults to default">
                </div>
                <div class="button-group">
                    <button class="btn-secondary" onclick="cancelDialog()">Cancel</button>
                    <button class="btn-primary" onclick="submitDialog()">Connect</button>
                </div>
            </div>

            <!-- Create Sandbox tab content -->
            <div id="create-tab" class="tab-content">
                <div class="form-group">
                    <label for="sandbox-namespace">Namespace</label>
                    <input type="text" id="sandbox-namespace" value="sandbox" placeholder="Defaults to sandbox">
                </div>
                <div class="form-group">
                    <label for="sandbox-name">Name</label>
                    <input type="text" id="sandbox-name" placeholder="Defaults to random UUID">
                </div>
                <div class="form-group">
                    <label for="sandbox-tenant">Tenant ID</label>
                    <input type="text" id="sandbox-tenant" value="default" placeholder="Defaults to default">
                </div>
                <div class="button-group">
                    <button class="btn-secondary" onclick="cancelDialog()">Cancel</button>
                    <button class="btn-create" id="submit-sandbox-btn" onclick="submitSandboxCreation()">Create &amp; Connect</button>
                </div>
            </div>
        </div>
    </div>

    <script src="%s/terminal/static/xterm.js"></script>
    <script src="%s/terminal/static/xterm-addon-fit.js"></script>
    <script>
        // Generate UUID
        const jobsApiUrl = '%s/api/jobs';

        function decodeBase64URL(input) {
            const base64 = input.replace(/-/g, '+').replace(/_/g, '/');
            const padded = base64 + '='.repeat((4 - base64.length %% 4) %% 4);
            return atob(padded);
        }

        function parseTenantFromJWT(token) {
            if (!token) {
                return '';
            }
            try {
                const parts = token.split('.');
                if (parts.length !== 3) {
                    return '';
                }
                const payload = JSON.parse(decodeBase64URL(parts[1]));
                return (payload && typeof payload.sub === 'string') ? payload.sub : '';
            } catch (e) {
                return '';
            }
        }

        function generateUUID() {
            return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
                const r = Math.random() * 16 | 0;
                const v = c === 'x' ? r : (r & 0x3 | 0x8);
                return v.toString(16);
            });
        }

        // Switch tab
        function switchTab(tabName) {
            // Update tab button states
            const tabs = document.querySelectorAll('.tab');
            tabs.forEach(tab => tab.classList.remove('active'));
            event.target.classList.add('active');

            // Update content area
            const connectTab = document.getElementById('connect-tab');
            const createTab = document.getElementById('create-tab');

            if (tabName === 'connect') {
                connectTab.classList.add('active');
                createTab.classList.remove('active');
            } else if (tabName === 'create') {
                connectTab.classList.remove('active');
                createTab.classList.add('active');

                // Auto-generate UUID when switching to create tab
                const nameInput = document.getElementById('sandbox-name');
                if (!nameInput.value) {
                    nameInput.value = generateUUID();
                }
            }
        }

        // Submit sandbox creation
        async function submitSandboxCreation() {
            const namespace = document.getElementById('sandbox-namespace').value.trim() || 'sandbox';
            const name = document.getElementById('sandbox-name').value.trim() || generateUUID();
            const tenant = document.getElementById('sandbox-tenant').value.trim() || 'default';
            const submitBtn = document.getElementById('submit-sandbox-btn');

            try {
                // Disable button and show loading state
                submitBtn.disabled = true;
                submitBtn.textContent = '⏳ Creating...';

                // Get current token
                const currentParams = new URLSearchParams(window.location.search);
                const token = currentParams.get('token');

                // Build request payload
                const payload = {
                    entrypoint: 'python3 -m yr.cli.scripts --user ' + tenant + ' sandbox create --name ' + name + ' --namespace ' + namespace,
                    runtime_env: {
                        working_dir: '/tmp',
                        env_vars: {
                            'YR_JWT_TOKEN': token || ''
                        }
                    }
                };

                // Build request options
                const fetchOptions = {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(payload)
                };

                if (token) {
                    fetchOptions.headers['X-Auth'] = token;
                }

                // Call job creation API
                const response = await fetch(jobsApiUrl, fetchOptions);

                if (!response.ok) {
                    throw new Error('Failed to create job: ' + response.status);
                }

                const result = await response.json();

                // Check returned submission_id
                if (!result.submission_id) {
                    throw new Error('submission_id not found in API response');
                }

                const submissionId = result.submission_id;
                submitBtn.textContent = '⏳ Waiting...';

                // Poll job status
                await pollJobStatus(submissionId, namespace, name, tenant, token);

            } catch (error) {
                console.error('Failed to create sandbox:', error);
                alert('Failed to create sandbox: ' + error.message);

                // Restore button state
                submitBtn.disabled = false;
                submitBtn.textContent = 'Create & Connect';
            }
        }

        // Poll job status
        async function pollJobStatus(submissionId, namespace, name, tenant, token) {
            const maxAttempts = 60; // max poll attempts
            const pollInterval = 2000; // poll every 2 seconds
            let attempts = 0;

            const poll = async () => {
                try {
                    // Build request options
                    const fetchOptions = {};
                    if (token) {
                        fetchOptions.headers = {
                            'X-Auth': token
                        };
                    }

                    // Query job status
                    const response = await fetch('%s/api/jobs/' + encodeURIComponent(submissionId), fetchOptions);

                    if (!response.ok) {
                        throw new Error('Failed to query job status: ' + response.status);
                    }

                    const jobInfo = await response.json();
                    const status = jobInfo.status;

                    if (status === 'SUCCEEDED') {
                        // Succeeded, redirect to web terminal
                        const instanceId = namespace + '-' + name;
                        const params = new URLSearchParams();
                        params.set('instance', instanceId);
                        params.set('tenant_id', tenant);
                        if (token) {
                            params.set('token', token);
                        }
                        window.location.search = params.toString();
                        return;
                    } else if (status === 'FAILED') {
                        // Failed
                        document.getElementById('custom-dialog-overlay').style.display = 'none';
                        alert('Sandbox creation failed: job execution failed\n' + (jobInfo.message || ''));
                        document.getElementById('submit-sandbox-btn').disabled = false;
                        document.getElementById('submit-sandbox-btn').textContent = 'Create & Connect';
                        return;
                    } else if (status === 'STOPPED') {
                        // Stopped
                        document.getElementById('custom-dialog-overlay').style.display = 'none';
                        alert('Sandbox creation failed: job was stopped\n' + (jobInfo.message || ''));
                        document.getElementById('submit-sandbox-btn').disabled = false;
                        document.getElementById('submit-sandbox-btn').textContent = 'Create & Connect';
                        return;
                    } else if (status === 'PENDING' || status === 'RUNNING') {
                        // Still running, continue polling
                        attempts++;
                        if (attempts >= maxAttempts) {
                            throw new Error('Timed out waiting. Check instance list later.');
                        }
                        setTimeout(poll, pollInterval);
                        return;
                    } else {
                        // Unknown status
                        throw new Error('Unknown job status: ' + status);
                    }
                } catch (error) {
                    document.getElementById('custom-dialog-overlay').style.display = 'none';
                    alert('Failed to query job status: ' + error.message);
                    document.getElementById('submit-sandbox-btn').disabled = false;
                    document.getElementById('submit-sandbox-btn').textContent = 'Create & Connect';
                }
            };

            // Start polling
            poll();
        }

        // Show custom dialog
        function showCustomDialog() {
            const overlay = document.getElementById('custom-dialog-overlay');
            overlay.style.display = 'flex';
            document.getElementById('dialog-instance').focus();

            // Support Enter key to submit
            const inputs = document.querySelectorAll('#custom-dialog input');
            inputs.forEach(input => {
                input.addEventListener('keypress', (e) => {
                    if (e.key === 'Enter') {
                        submitDialog();
                    }
                });
            });
        }

        // Cancel dialog
        function cancelDialog() {
            document.getElementById('terminal').innerHTML =
                '<div style="color: #f44336; padding: 20px; text-align: center;">' +
                '<h2>⚠️ No Instance Specified</h2>' +
                '<p>Please refresh the page and re-enter connection details</p>' +
                '</div>';
            document.getElementById('status-text').textContent = 'No instance specified';
            document.getElementById('custom-dialog-overlay').style.display = 'none';
        }

        // Submit dialog
        function submitDialog() {
            const instance = document.getElementById('dialog-instance').value.trim();
            const tenant = document.getElementById('dialog-tenant').value.trim() || 'default';

            if (!instance) {
                alert('Please enter an instance name or ID');
                document.getElementById('dialog-instance').focus();
                return;
            }

            // Build new URL params, preserve existing token
            const currentParams = new URLSearchParams(window.location.search);
            const token = currentParams.get('token');

            const params = new URLSearchParams();
            params.set('instance', instance);
            params.set('tenant_id', tenant);
            if (token) {
                params.set('token', token);
            }

            // Redirect to URL with params
            window.location.search = params.toString();
        }

        // Toggle sidebar visibility
        function toggleSidebar() {
            const mainContainer = document.getElementById('main-container');
            const toggleBtn = document.getElementById('toggle-sidebar-btn');
            const isHidden = mainContainer.classList.toggle('sidebar-hidden');
            const tip = isHidden ? 'Show instance list' : 'Hide instance list';
            toggleBtn.title = tip;
            toggleBtn.setAttribute('aria-label', tip);

            // Trigger terminal resize after layout changes
            setTimeout(() => {
                window.dispatchEvent(new Event('resize'));
            }, 0);
        }
    </script>
    <script>
        // Pagination config
        let currentPage = 1;
        let pageSize = 10;
        let totalInstances = 0;
        let allInstances = [];

        async function deleteInstance(instanceId, token) {
            if (!instanceId) {
                return;
            }
            if (!confirm('Delete instance: ' + instanceId + ' ?')) {
                return;
            }

            try {
                const payload = {
                    entrypoint: 'python3 -m yr.cli.scripts sandbox ' + instanceId,
                    runtime_env: {
                        working_dir: '/tmp'
                    }
                };

                const fetchOptions = {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(payload)
                };
                if (token) {
                    fetchOptions.headers['X-Auth'] = token;
                }

                const response = await fetch(jobsApiUrl, fetchOptions);
                if (!response.ok) {
                    throw new Error('Failed to submit delete job: ' + response.status);
                }

                const result = await response.json();
                alert('Delete job submitted' + (result && result.submission_id ? (': ' + result.submission_id) : ''));

                // Refresh list after delete request is submitted
                loadInstances(currentPage);
            } catch (error) {
                console.error('Failed to delete instance:', error);
                alert('Failed to delete instance: ' + error.message);
            }
        }

        // Load instance list
        async function loadInstances(page = 1) {
            try {
                // Get tenant_id and token params
                const params = new URLSearchParams(window.location.search);
                const tenantId = params.get('tenant_id') || 'default';
                const token = params.get('token') || '';

                // Build request options
                const fetchOptions = {};
                if (token) {
                    fetchOptions.headers = {
                        'X-Auth': token
                    };
                }

                const response = await fetch('%s/api/instances?tenant_id=' + encodeURIComponent(tenantId), fetchOptions);
                const instances = await response.json();

                // Save all instance data
                allInstances = instances;
                totalInstances = instances.length;
                currentPage = page;

                const listContainer = document.getElementById('instance-list');

                // Clear list
                listContainer.innerHTML = '';

                // Get current instance from URL params
                const currentInstance = params.get('instance') || '';

                // Show message if no instances
                if (instances.length === 0) {
                    listContainer.innerHTML = '<div style="padding: 20px; text-align: center; color: #888; font-size: 12px;">No instances</div>';
                    updatePaginationUI();
                    return;
                }

                // Calculate pagination
                const totalPages = Math.ceil(totalInstances / pageSize);
                const startIndex = (currentPage - 1) * pageSize;
                const endIndex = Math.min(startIndex + pageSize, totalInstances);
                const pageInstances = instances.slice(startIndex, endIndex);

                // Render instance list
                pageInstances.forEach(instance => {
                    const item = document.createElement('div');
                    item.className = 'instance-item';
                    if (instance.id === currentInstance) {
                        item.classList.add('active');
                    }

                    const headDiv = document.createElement('div');
                    headDiv.className = 'instance-head';

                    const idDiv = document.createElement('div');
                    idDiv.className = 'instance-id';
                    idDiv.textContent = instance.id;

                    const deleteBtn = document.createElement('button');
                    deleteBtn.className = 'instance-delete-btn';
                    deleteBtn.textContent = '🗑';
                    deleteBtn.title = 'Delete instance';
                    deleteBtn.addEventListener('click', (e) => {
                        e.stopPropagation();
                        deleteInstance(instance.id, token);
                    });

                    headDiv.appendChild(idDiv);
                    headDiv.appendChild(deleteBtn);

                    const statusDiv = document.createElement('div');
                    statusDiv.className = 'instance-status';
                    const status = instance.status || 'unknown';
                    const errorMessage = instance.error || '';
                    statusDiv.textContent = '● ' + status;
                    // Simple status color check
                    const isRunning = status.toLowerCase().includes('running') || status.toLowerCase().includes('ready');
                    if (isRunning) {
                        statusDiv.classList.add('running');
                    } else if (status.toLowerCase().includes('stop') || status.toLowerCase().includes('error')) {
                        statusDiv.classList.add('stopped');
                    }

                    // For non-running instances, show error detail in hover tooltip
                    if (!isRunning && errorMessage) {
                        statusDiv.classList.add('with-error');
                        statusDiv.title = errorMessage;
                    }

                    item.appendChild(headDiv);
                    item.appendChild(statusDiv);

                    // Click to switch instance
                    item.addEventListener('click', () => {
                        switchInstance(instance.id);
                    });

                    listContainer.appendChild(item);
                });

                // Update pagination UI
                updatePaginationUI();

            } catch (error) {
                console.error('Failed to load instances:', error);
                const listContainer = document.getElementById('instance-list');
                listContainer.innerHTML = '<div style="padding: 20px; text-align: center; color: #f44336; font-size: 12px;">Load failed</div>';
                updatePaginationUI();
            }
        }

        // Update pagination UI
        function updatePaginationUI() {
            const totalPages = Math.ceil(totalInstances / pageSize);
            const startIndex = (currentPage - 1) * pageSize + 1;
            const endIndex = Math.min(currentPage * pageSize, totalInstances);

            // Update page info
            const pageInfoText = document.getElementById('page-info-text');
            if (totalInstances === 0) {
                pageInfoText.textContent = 'No data';
            } else {
                pageInfoText.textContent = startIndex + '-' + endIndex + ' / ' + totalInstances;
            }

            // Update button states
            document.getElementById('first-page-btn').disabled = currentPage === 1;
            document.getElementById('prev-page-btn').disabled = currentPage === 1;
            document.getElementById('next-page-btn').disabled = currentPage >= totalPages;
            document.getElementById('last-page-btn').disabled = currentPage >= totalPages;
        }

        // Switch instance
        function switchInstance(instanceId) {
            // Close existing WebSocket before switching instances
            if (ws && ws.readyState === WebSocket.OPEN) {
                ws.close();
            }
            const params = new URLSearchParams(window.location.search);
            if (instanceId) {
                params.set('instance', instanceId);
            } else {
                params.delete('instance');
            }
            // Reload page with new instance param
            window.location.search = params.toString();
        }

        // Initialize
        document.addEventListener('DOMContentLoaded', () => {
            // Sidebar toggle button event
            document.getElementById('toggle-sidebar-btn').addEventListener('click', () => {
                toggleSidebar();
            });

            // Check if instance param exists
            const params = new URLSearchParams(window.location.search);
            const tokenFromUrl = params.get('token');
            const tenantFromToken = parseTenantFromJWT(tokenFromUrl);
            if (tenantFromToken) {
                const dialogTenantInput = document.getElementById('dialog-tenant');
                if (dialogTenantInput) {
                    dialogTenantInput.value = tenantFromToken;
                }
                const sandboxTenantInput = document.getElementById('sandbox-tenant');
                if (sandboxTenantInput) {
                    sandboxTenantInput.value = tenantFromToken;
                }
            }
            const currentInstance = params.get('instance');

            // No instance param, show dialog
            if (!currentInstance) {
                showCustomDialog();
                return; // Stop further init, wait for user input
            }

            loadInstances();

            // Manual instance input button event
            document.getElementById('add-instance-btn').addEventListener('click', () => {
                showCustomDialog();
            });

            // Refresh button event
            document.getElementById('refresh-btn').addEventListener('click', () => {
                loadInstances(currentPage);
            });

            // Pagination button events
            document.getElementById('first-page-btn').addEventListener('click', () => {
                loadInstances(1);
            });

            document.getElementById('prev-page-btn').addEventListener('click', () => {
                if (currentPage > 1) {
                    loadInstances(currentPage - 1);
                }
            });

            document.getElementById('next-page-btn').addEventListener('click', () => {
                const totalPages = Math.ceil(totalInstances / pageSize);
                if (currentPage < totalPages) {
                    loadInstances(currentPage + 1);
                }
            });

            document.getElementById('last-page-btn').addEventListener('click', () => {
                const totalPages = Math.ceil(totalInstances / pageSize);
                loadInstances(totalPages);
            });

            // Initialize Terminal (only when container ID is available)
            const term = new Terminal({
                cursorBlink: true,
                fontSize: 14,
                fontFamily: '"Cascadia Code", "Courier New", monospace',
                theme: {
                    background: '#1e1e1e',
                    foreground: '#d4d4d4',
                    cursor: '#aeafad',
                    black: '#000000',
                    red: '#cd3131',
                    green: '#0dbc79',
                    yellow: '#e5e510',
                    blue: '#2472c8',
                    magenta: '#bc3fbc',
                    cyan: '#11a8cd',
                    white: '#e5e5e5',
                    brightBlack: '#666666',
                    brightRed: '#f14c4c',
                    brightGreen: '#23d18b',
                    brightYellow: '#f5f543',
                    brightBlue: '#3b8eea',
                    brightMagenta: '#d670d6',
                    brightCyan: '#29b8db',
                    brightWhite: '#ffffff'
                }
            });

            const fitAddon = new FitAddon.FitAddon();
            term.loadAddon(fitAddon);

            term.open(document.getElementById('terminal'));
            fitAddon.fit();

            // Initialize WebSocket connection
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            // Extract token from URL but omit it from the WebSocket URL (prevents log leakage).
            // Browser WebSocket does not support custom headers; use Sec-WebSocket-Protocol instead.
            const _wsParams = new URLSearchParams(window.location.search);
            const _wsToken = _wsParams.get('token');
            _wsParams.delete('token');
            // Add unique query suffix to avoid client/proxy caching surprises across tabs
            const uniqueQuerySuffix = Date.now().toString() + '-' + Math.random().toString(36).substring(2, 9);
            _wsParams.set('_t', uniqueQuerySuffix);
            const wsUrl = protocol + '//' + window.location.host + '%s/terminal/ws' + (_wsParams.toString() ? '?' + _wsParams.toString() : '');
            document.getElementById('ws-url').textContent = wsUrl;

            // Pass token as subprotocol (backend echoes it back to complete the handshake)
            // IMPORTANT: pass raw token only, do not append suffixes
            const subprotocols = _wsToken ? [_wsToken] : [];
            const ws = new WebSocket(wsUrl, subprotocols);
            ws.binaryType = 'arraybuffer';

            function sendTerminalSize() {
                if (ws.readyState !== WebSocket.OPEN) {
                    return;
                }
                fitAddon.fit();
                const cols = term.cols;
                const rows = term.rows;
                if (cols > 0 && rows > 0) {
                    console.log('Sending terminal size:', cols, 'x', rows);
                    ws.send('RESIZE:' + cols + ':' + rows);
                }
            }

            window.addEventListener('resize', () => {
                sendTerminalSize();
            });

            ws.onopen = () => {
                document.getElementById('status-text').textContent = 'Connected';
                document.getElementById('status-indicator').classList.add('connected');

                // Send size immediately, then retry once in next frame and once after short delay.
                sendTerminalSize();
                requestAnimationFrame(() => {
                    sendTerminalSize();
                });
                setTimeout(() => {
                    sendTerminalSize();
                }, 120);

                // Periodic heartbeat to detect connection issues
                // Check WebSocket state every 10 seconds
                window.terminalHeartbeat = setInterval(() => {
                    if (ws.readyState !== WebSocket.OPEN) {
                        console.log('WebSocket heartbeat: connection lost, state=', ws.readyState);
                        clearInterval(window.terminalHeartbeat);
                        term.write('\r\n\x1b[1;31m[Connection lost - please refresh the page to reconnect]\x1b[0m\r\n');
                        // Note: Don't refresh page as it would lose terminal context
                        // Backend will clean up resources via gRPC keepalive timeout
                    }
                }, 10000);

                term.focus();
            };

            ws.onmessage = (event) => {
                let data;
                if (event.data instanceof ArrayBuffer) {
                    data = new Uint8Array(event.data);
                    term.write(data);
                } else {
                    term.write(event.data);
                }
            };

            ws.onerror = (error) => {
                console.error('WebSocket error:', error);
                term.write('\r\n\x1b[1;31m[Connection Error]\x1b[0m\r\n');
            };

            // Note: Do NOT explicitly close WebSocket on page unload.
            // Let the browser handle connection closure naturally to avoid
            // potentially affecting other WebSocket connections sharing the same HTTP/2 connection.
            // Browser will properly clean up when the page is destroyed.

            ws.onclose = (event) => {
                console.log('WebSocket closed:', event.code, event.reason);

                // Clear heartbeat interval
                if (window.terminalHeartbeat) {
                    clearInterval(window.terminalHeartbeat);
                }

                document.getElementById('status-text').textContent = 'Disconnected';
                document.getElementById('status-indicator').classList.remove('connected');
                document.getElementById('status-indicator').classList.add('disconnected');

                // If closed abnormally (1006), inform user but don't refresh
                if (event.code === 1006) {
                    term.write('\r\n\x1b[1;31m[Connection lost - please refresh the page to reconnect]\x1b[0m\r\n');
                    return;
                }

                term.write('\r\n\x1b[1;33m[Connection Closed]\x1b[0m\r\n');
            };

            term.onData((data) => {
                if (ws.readyState === WebSocket.OPEN) {
                    ws.send(data);
                }
            });

            term.onResize(({ cols, rows }) => {
                console.log('Terminal resized:', cols, 'x', rows);
                if (ws.readyState === WebSocket.OPEN) {
                    ws.send('RESIZE:' + cols + ':' + rows);
                }
            });

            term.focus();
        }); // End DOMContentLoaded
    </script>
</body>
</html>`, pathPrefix, pathPrefix, pathPrefix, pathPrefix, pathPrefix, pathPrefix, pathPrefix, pathPrefix)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
