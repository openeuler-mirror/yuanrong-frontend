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
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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
	// Authenticate JWT token from query parameter or header
	if config.GetConfig().IamConfig.EnableFuncTokenAuth {
		token := r.URL.Query().Get("token")
		if token == "" {
			token = r.Header.Get("X-Auth")
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

		log.GetLogger().Infof("WebSocket JWT authentication passed, role: %s, tenant: %s",
			parsedJWT.Payload.Role, parsedJWT.Payload.Sub)
	}

	// Log client certificate info if TLS is enabled (verification already done at TLS handshake)
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		clientCert := r.TLS.PeerCertificates[0]
		log.GetLogger().Infof("Client connected with certificate: Subject=%s, Issuer=%s",
			clientCert.Subject.String(), clientCert.Issuer.String())
	}

	conn, err := upgrader.Upgrade(w, r, nil)
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
	tenantID := query.Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}

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
	grpcConn, err := grpc.NewClient(info.ProxyGrpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.GetLogger().Infof("Failed to connect to executor: %v", err)
		return
	}
	defer grpcConn.Close()

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

	done := make(chan struct{})

	// Read output from gRPC and send to WebSocket
	go func() {
		defer func() {
			select {
			case <-done:
			default:
				close(done)
			}
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
		instance := map[string]interface{}{
			"id":       inst.InstanceID,
			"function": inst.Function,
			"status":   inst.InstanceStatus.Msg,
		}
		instances = append(instances, instance)
	}

	// Return instance list
	if err := json.NewEncoder(w).Encode(instances); err != nil {
		log.GetLogger().Infof("Error encoding instances: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
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
            justify-content: space-between;
            align-items: center;
        }
        #header .left-section {
            display: flex;
            align-items: center;
            gap: 16px;
        }
        #header h1 {
            margin: 0;
            font-size: 16px;
            font-weight: normal;
        }
        .back-link {
            color: #ccc;
            text-decoration: none;
            opacity: 0.8;
            transition: opacity 0.2s;
            font-size: 14px;
        }
        .back-link:hover {
            opacity: 1;
        }
        #status {
            display: flex;
            align-items: center;
            gap: 10px;
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
        }
        .instance-item .instance-status {
            font-size: 11px;
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
        /* 自定义对话框样式 */
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
        <div class="left-section">
            <a href="%s/" class="back-link">← 首页</a>
            <h1>🖥️ Remote Exec Terminal</h1>
        </div>
        <div id="status">
            <span id="status-text">Connecting...</span>
            <div class="status-indicator" id="status-indicator"></div>
        </div>
    </div>
    <div id="main-container">
        <div id="sidebar">
            <div id="sidebar-header">
                <h2>实例列表</h2>
                <button id="refresh-btn" title="刷新实例列表">🔄</button>
            </div>
            <div id="instance-list">
                <div style="padding: 20px; text-align: center; color: #888; font-size: 12px;">
                    加载中...
                </div>
            </div>
            <div id="pagination">
                <div class="page-info">
                    <span id="page-info-text">-</span>
                </div>
                <div class="page-controls">
                    <button id="first-page-btn" title="首页">«</button>
                    <button id="prev-page-btn" title="上一页">‹</button>
                    <button id="next-page-btn" title="下一页">›</button>
                    <button id="last-page-btn" title="末页">»</button>
                </div>
            </div>
            <div id="sidebar-footer">
                <button id="add-instance-btn">✏️ 手动输入实例ID</button>
            </div>
        </div>
        <div id="terminal-container">
            <div id="terminal"></div>
        </div>
    </div>
    <div id="footer">
        Press Ctrl+C to interrupt | Connection: <span id="ws-url"></span>
    </div>

    <!-- 自定义输入对话框 -->
    <div id="custom-dialog-overlay">
        <div id="custom-dialog">
            <h2>🖥️ 连接配置</h2>

            <!-- 选项卡 -->
            <div class="tab-container">
                <button class="tab active" onclick="switchTab('connect')">连接实例</button>
                <button class="tab" onclick="switchTab('create')">创建Sandbox</button>
            </div>

            <!-- 连接实例选项卡内容 -->
            <div id="connect-tab" class="tab-content active">
                <div class="form-group">
                    <label for="dialog-instance">实例名称或ID *</label>
                    <input type="text" id="dialog-instance" placeholder="请输入实例名称或实例ID">
                </div>
                <div class="form-group">
                    <label for="dialog-tenant">租户ID（Tenant ID）</label>
                    <input type="text" id="dialog-tenant" value="default" placeholder="默认为 default">
                </div>
                <div class="button-group">
                    <button class="btn-secondary" onclick="cancelDialog()">取消</button>
                    <button class="btn-primary" onclick="submitDialog()">连接</button>
                </div>
            </div>

            <!-- 创建Sandbox选项卡内容 -->
            <div id="create-tab" class="tab-content">
                <div class="form-group">
                    <label for="sandbox-namespace">Namespace</label>
                    <input type="text" id="sandbox-namespace" value="sandbox" placeholder="默认为 sandbox">
                </div>
                <div class="form-group">
                    <label for="sandbox-name">Name</label>
                    <input type="text" id="sandbox-name" placeholder="默认随机生成UUID">
                </div>
                <div class="form-group">
                    <label for="sandbox-tenant">租户ID（Tenant ID）</label>
                    <input type="text" id="sandbox-tenant" value="default" placeholder="默认为 default">
                </div>
                <div class="button-group">
                    <button class="btn-secondary" onclick="cancelDialog()">取消</button>
                    <button class="btn-create" id="submit-sandbox-btn" onclick="submitSandboxCreation()">创建并连接</button>
                </div>
            </div>
        </div>
    </div>

    <script src="%s/terminal/static/xterm.js"></script>
    <script src="%s/terminal/static/xterm-addon-fit.js"></script>
    <script>
        // 生成UUID
        function generateUUID() {
            return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
                const r = Math.random() * 16 | 0;
                const v = c === 'x' ? r : (r & 0x3 | 0x8);
                return v.toString(16);
            });
        }

        // 切换选项卡
        function switchTab(tabName) {
            // 更新选项卡按钮状态
            const tabs = document.querySelectorAll('.tab');
            tabs.forEach(tab => tab.classList.remove('active'));
            event.target.classList.add('active');

            // 更新内容区域
            const connectTab = document.getElementById('connect-tab');
            const createTab = document.getElementById('create-tab');

            if (tabName === 'connect') {
                connectTab.classList.add('active');
                createTab.classList.remove('active');
            } else if (tabName === 'create') {
                connectTab.classList.remove('active');
                createTab.classList.add('active');

                // 切换到创建选项卡时，自动生成UUID
                const nameInput = document.getElementById('sandbox-name');
                if (!nameInput.value) {
                    nameInput.value = generateUUID();
                }
            }
        }

        // 提交创建Sandbox
        async function submitSandboxCreation() {
            const namespace = document.getElementById('sandbox-namespace').value.trim() || 'sandbox';
            const name = document.getElementById('sandbox-name').value.trim() || generateUUID();
            const tenant = document.getElementById('sandbox-tenant').value.trim() || 'default';
            const submitBtn = document.getElementById('submit-sandbox-btn');

            try {
                // 禁用按钮并显示加载状态
                submitBtn.disabled = true;
                submitBtn.textContent = '⏳ 创建中...';

                // 获取当前token
                const currentParams = new URLSearchParams(window.location.search);
                const token = currentParams.get('token');

                // 构建请求payload
                const payload = {
                    entrypoint: 'python -m yr.sandbox.sandbox --name ' + name + ' --namespace ' + namespace,
                    runtime_env: {
                        working_dir: '/tmp'
                    }
                };

                // 构建请求选项
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

                // 调用创建作业API
                const response = await fetch('%s/api/jobs', fetchOptions);

                if (!response.ok) {
                    throw new Error('创建作业失败: ' + response.status);
                }

                const result = await response.json();

                // 检查返回的submission_id
                if (!result.submission_id) {
                    throw new Error('API返回的数据中没有找到submission_id');
                }

                const submissionId = result.submission_id;
                submitBtn.textContent = '⏳ 等待就绪...';

                // 轮询作业状态
                await pollJobStatus(submissionId, namespace, name, tenant, token);

            } catch (error) {
                console.error('创建Sandbox失败:', error);
                alert('创建Sandbox失败: ' + error.message);

                // 恢复按钮状态
                submitBtn.disabled = false;
                submitBtn.textContent = '创建并连接';
            }
        }

        // 轮询作业状态
        async function pollJobStatus(submissionId, namespace, name, tenant, token) {
            const maxAttempts = 60; // 最多轮询60次
            const pollInterval = 2000; // 每2秒轮询一次
            let attempts = 0;

            const poll = async () => {
                try {
                    // 构建请求选项
                    const fetchOptions = {};
                    if (token) {
                        fetchOptions.headers = {
                            'X-Auth': token
                        };
                    }

                    // 查询作业状态
                    const response = await fetch('%s/api/jobs/' + encodeURIComponent(submissionId), fetchOptions);

                    if (!response.ok) {
                        throw new Error('查询作业状态失败: ' + response.status);
                    }

                    const jobInfo = await response.json();
                    const status = jobInfo.status;

                    if (status === 'SUCCEEDED') {
                        // 执行成功，跳转到webterminal
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
                        // 执行失败
                        document.getElementById('custom-dialog-overlay').style.display = 'none';
                        alert('Sandbox创建失败: 作业执行失败\n' + (jobInfo.message || ''));
                        document.getElementById('submit-sandbox-btn').disabled = false;
                        document.getElementById('submit-sandbox-btn').textContent = '创建并连接';
                        return;
                    } else if (status === 'STOPPED') {
                        // 被停止
                        document.getElementById('custom-dialog-overlay').style.display = 'none';
                        alert('Sandbox创建失败: 作业被停止\n' + (jobInfo.message || ''));
                        document.getElementById('submit-sandbox-btn').disabled = false;
                        document.getElementById('submit-sandbox-btn').textContent = '创建并连接';
                        return;
                    } else if (status === 'PENDING' || status === 'RUNNING') {
                        // 正在执行中，继续轮询
                        attempts++;
                        if (attempts >= maxAttempts) {
                            throw new Error('等待超时，请稍后在实例列表中查看');
                        }
                        setTimeout(poll, pollInterval);
                        return;
                    } else {
                        // 未知状态
                        throw new Error('未知的作业状态: ' + status);
                    }
                } catch (error) {
                    document.getElementById('custom-dialog-overlay').style.display = 'none';
                    alert('查询作业状态失败: ' + error.message);
                    document.getElementById('submit-sandbox-btn').disabled = false;
                    document.getElementById('submit-sandbox-btn').textContent = '创建并连接';
                }
            };

            // 开始轮询
            poll();
        }

        // 显示自定义对话框
        function showCustomDialog() {
            const overlay = document.getElementById('custom-dialog-overlay');
            overlay.style.display = 'flex';
            document.getElementById('dialog-instance').focus();

            // 支持回车键提交
            const inputs = document.querySelectorAll('#custom-dialog input');
            inputs.forEach(input => {
                input.addEventListener('keypress', (e) => {
                    if (e.key === 'Enter') {
                        submitDialog();
                    }
                });
            });
        }

        // 取消对话框
        function cancelDialog() {
            document.getElementById('terminal').innerHTML =
                '<div style="color: #f44336; padding: 20px; text-align: center;">' +
                '<h2>⚠️ 未指定实例</h2>' +
                '<p>请刷新页面重新输入连接信息</p>' +
                '</div>';
            document.getElementById('status-text').textContent = 'No instance specified';
            document.getElementById('custom-dialog-overlay').style.display = 'none';
        }

        // 提交对话框
        function submitDialog() {
            const instance = document.getElementById('dialog-instance').value.trim();
            const tenant = document.getElementById('dialog-tenant').value.trim() || 'default';

            if (!instance) {
                alert('请输入实例名称或ID');
                document.getElementById('dialog-instance').focus();
                return;
            }

            // 构建新的URL参数，保留现有的token
            const currentParams = new URLSearchParams(window.location.search);
            const token = currentParams.get('token');

            const params = new URLSearchParams();
            params.set('instance', instance);
            params.set('tenant_id', tenant);
            if (token) {
                params.set('token', token);
            }

            // 重定向到带有参数的URL
            window.location.search = params.toString();
        }
    </script>
    <script>
        // 分页配置
        let currentPage = 1;
        let pageSize = 10;
        let totalInstances = 0;
        let allInstances = [];

        // 加载实例列表
        async function loadInstances(page = 1) {
            try {
                // 获取 tenant_id 和 token 参数
                const params = new URLSearchParams(window.location.search);
                const tenantId = params.get('tenant_id') || 'default';
                const token = params.get('token') || '';

                // 构建请求选项
                const fetchOptions = {};
                if (token) {
                    fetchOptions.headers = {
                        'X-Auth': token
                    };
                }

                const response = await fetch('%s/api/instances?tenant_id=' + encodeURIComponent(tenantId), fetchOptions);
                const instances = await response.json();

                // 保存所有实例数据
                allInstances = instances;
                totalInstances = instances.length;
                currentPage = page;

                const listContainer = document.getElementById('instance-list');

                // 清空列表
                listContainer.innerHTML = '';

                // 获取当前实例（从URL参数）
                const currentInstance = params.get('instance') || '';

                // 如果没有实例，显示提示
                if (instances.length === 0) {
                    listContainer.innerHTML = '<div style="padding: 20px; text-align: center; color: #888; font-size: 12px;">暂无实例</div>';
                    updatePaginationUI();
                    return;
                }

                // 计算分页
                const totalPages = Math.ceil(totalInstances / pageSize);
                const startIndex = (currentPage - 1) * pageSize;
                const endIndex = Math.min(startIndex + pageSize, totalInstances);
                const pageInstances = instances.slice(startIndex, endIndex);

                // 渲染实例列表
                pageInstances.forEach(instance => {
                    const item = document.createElement('div');
                    item.className = 'instance-item';
                    if (instance.id === currentInstance) {
                        item.classList.add('active');
                    }

                    const idDiv = document.createElement('div');
                    idDiv.className = 'instance-id';
                    idDiv.textContent = instance.id;

                    const statusDiv = document.createElement('div');
                    statusDiv.className = 'instance-status';
                    const status = instance.status || 'unknown';
                    statusDiv.textContent = '● ' + status;
                    // 简单的状态颜色判断
                    if (status.toLowerCase().includes('running') || status.toLowerCase().includes('ready')) {
                        statusDiv.classList.add('running');
                    } else if (status.toLowerCase().includes('stop') || status.toLowerCase().includes('error')) {
                        statusDiv.classList.add('stopped');
                    }

                    item.appendChild(idDiv);
                    item.appendChild(statusDiv);

                    // 点击切换实例
                    item.addEventListener('click', () => {
                        switchInstance(instance.id);
                    });

                    listContainer.appendChild(item);
                });

                // 更新分页UI
                updatePaginationUI();

            } catch (error) {
                console.error('Failed to load instances:', error);
                const listContainer = document.getElementById('instance-list');
                listContainer.innerHTML = '<div style="padding: 20px; text-align: center; color: #f44336; font-size: 12px;">加载失败</div>';
                updatePaginationUI();
            }
        }

        // 更新分页UI
        function updatePaginationUI() {
            const totalPages = Math.ceil(totalInstances / pageSize);
            const startIndex = (currentPage - 1) * pageSize + 1;
            const endIndex = Math.min(currentPage * pageSize, totalInstances);

            // 更新页面信息
            const pageInfoText = document.getElementById('page-info-text');
            if (totalInstances === 0) {
                pageInfoText.textContent = '无数据';
            } else {
                pageInfoText.textContent = startIndex + '-' + endIndex + ' / ' + totalInstances;
            }

            // 更新按钮状态
            document.getElementById('first-page-btn').disabled = currentPage === 1;
            document.getElementById('prev-page-btn').disabled = currentPage === 1;
            document.getElementById('next-page-btn').disabled = currentPage >= totalPages;
            document.getElementById('last-page-btn').disabled = currentPage >= totalPages;
        }

        // 切换实例
        function switchInstance(instanceId) {
            const params = new URLSearchParams(window.location.search);
            if (instanceId) {
                params.set('instance', instanceId);
            } else {
                params.delete('instance');
            }
            // 重新加载页面并带上新的实例参数
            window.location.search = params.toString();
        }

        // 初始化
        document.addEventListener('DOMContentLoaded', () => {
            // 检查是否有实例参数
            const params = new URLSearchParams(window.location.search);
            const currentInstance = params.get('instance');

            // 如果没有实例参数，显示自定义对话框要求用户输入
            if (!currentInstance) {
                showCustomDialog();
                return; // 停止后续初始化，等待用户输入
            }

            loadInstances();

            // 手动输入实例按钮事件
            document.getElementById('add-instance-btn').addEventListener('click', () => {
                showCustomDialog();
            });

            // 刷新按钮事件
            document.getElementById('refresh-btn').addEventListener('click', () => {
                loadInstances(currentPage);
            });

            // 分页按钮事件
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

            // 初始化 Terminal（只有在有容器ID时才执行）
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

            window.addEventListener('resize', () => {
            fitAddon.fit();
        });

            // 初始化 WebSocket 连接
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            // 保留所有 URL 参数（包括 token）
            const wsUrl = protocol + '//' + window.location.host + '%s/terminal/ws' + window.location.search;
            document.getElementById('ws-url').textContent = wsUrl;

            const ws = new WebSocket(wsUrl);
            ws.binaryType = 'arraybuffer';

            ws.onopen = () => {
                document.getElementById('status-text').textContent = 'Connected';
                document.getElementById('status-indicator').classList.add('connected');

                // 稍微延迟发送终端尺寸，确保后端已经初始化PTY
                setTimeout(() => {
                    if (ws.readyState === WebSocket.OPEN) {
                        const cols = term.cols;
                        const rows = term.rows;
                        console.log('Sending initial terminal size:', cols, 'x', rows);
                        ws.send('RESIZE:' + cols + ':' + rows);
                    }
                }, 100);

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
                term.write('\r\n\x1b[1;31m[Connection Error]\x1b[0m\r\n');
            };

            ws.onclose = () => {
                document.getElementById('status-text').textContent = 'Disconnected';
                document.getElementById('status-indicator').classList.remove('connected');
                document.getElementById('status-indicator').classList.add('disconnected');
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
        }); // 结束 DOMContentLoaded
    </script>
</body>
</html>`, pathPrefix, pathPrefix, pathPrefix, pathPrefix, pathPrefix, pathPrefix, pathPrefix, pathPrefix)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
