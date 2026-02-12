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

package webterm

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
	"frontend/pkg/frontend/common/util"
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
	InstanceID       string          `json:"instanceID"`       // Instance ID
	TenantID         string          `json:"tenantID"`         // Tenant ID
	ContainerID      string          `json:"containerID"`      // Container ID
	ProxyGrpcAddress string          `json:"proxyGrpcAddress"` // Proxy gRPC address
	FunctionProxyID  string          `json:"functionProxyID"`  // Function Proxy ID
	Function         string          `json:"function"`         // Function name
	RuntimeAddress   string          `json:"runtimeAddress"`   // Runtime address
	RuntimeID        string          `json:"runtimeID"`        // Runtime ID
	InstanceStatus   InstanceStatus  `json:"instanceStatus"`   // Instance status
	Resources        Resources       `json:"resources"`        // Resource configuration
	StartTime        string          `json:"startTime"`        // Start time
	RequestID        string          `json:"requestID"`        // Request ID
	ParentID         string          `json:"parentID"`         // Parent ID
	JobID            string          `json:"jobID"`            // Job ID
	NodeIP           string          `json:"nodeIP"`           // Node IP
	NodePort         string          `json:"nodePort"`         // Node port
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
		command = []string{"/bin/sh", "-c", cmdStr}
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
			"name":     inst.Function,
			"status":   inst.InstanceStatus.Msg,
			"nodeIP":   inst.NodeIP,
			"nodePort": inst.NodePort,
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
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Remote Exec Terminal</title>
    <meta charset="UTF-8">
    <!--
        This page uses xterm.js - Copyright (c) 2017-2022, The xterm.js authors
        Licensed under the MIT License - https://github.com/xtermjs/xterm.js
    -->
    <link rel="stylesheet" href="/terminal/static/xterm.css" />
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
        #header h1 {
            margin: 0;
            font-size: 16px;
            font-weight: normal;
        }
        #status {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        #container-selector {
            background: #3c3c3c;
            color: #d4d4d4;
            border: 1px solid #555;
            padding: 4px 8px;
            border-radius: 3px;
            font-size: 13px;
            cursor: pointer;
            outline: none;
        }
        #container-selector:hover {
            background: #4c4c4c;
        }
        #container-selector option {
            background: #3c3c3c;
        }
        #refresh-btn {
            background: #3c3c3c;
            color: #d4d4d4;
            border: 1px solid #555;
            padding: 4px 8px;
            border-radius: 3px;
            cursor: pointer;
            font-size: 14px;
            outline: none;
        }
        #refresh-btn:hover {
            background: #4c4c4c;
        }
        #refresh-btn:active {
            background: #2c2c2c;
        }
        .status-indicator {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: #666;
        }
        .status-indicator.connected {
            background: #4caf50;
            box-shadow: 0 0 5px #4caf50;
        }
        .status-indicator.disconnected {
            background: #f44336;
        }
        #terminal-container {
            flex: 1;
            padding: 10px;
            overflow: hidden;
        }
        #terminal {
            height: 100%;
        }
        #footer {
            background: #2d2d30;
            color: #888;
            padding: 5px 20px;
            border-top: 1px solid #3e3e42;
            font-size: 12px;
            text-align: center;
        }
    </style>
</head>
<body>
    <div id="header">
        <h1>🖥️ Remote Exec Terminal</h1>
        <div id="status">
            <select id="container-selector" title="Select instance">
                <option value="">Loading instances...</option>
            </select>
            <button id="refresh-btn" title="Refresh container list">🔄</button>
            <span id="status-text">Connecting...</span>
            <div class="status-indicator" id="status-indicator"></div>
        </div>
    </div>
    <div id="terminal-container">
        <div id="terminal"></div>
    </div>
    <div id="footer">
        Press Ctrl+C to interrupt | Connection: <span id="ws-url"></span>
    </div>

    <script src="/terminal/static/xterm.js"></script>
    <script src="/terminal/static/xterm-addon-fit.js"></script>
    <script>
        // 加载实例列表
        async function loadInstances() {
            try {
                const response = await fetch('/api/instances');
                const instances = await response.json();
                const selector = document.getElementById('container-selector');
                
                // 清空选项
                selector.innerHTML = '';
                
                // 获取当前实例（从URL参数）
                const params = new URLSearchParams(window.location.search);
                const currentInstance = params.get('instance') || '';
                
                // 只显示前10个实例
                const displayInstances = instances.slice(0, 10);
                
                // 添加实例选项
                displayInstances.forEach(instance => {
                    const option = document.createElement('option');
                    option.value = instance.id;
                    option.textContent = instance.name + ' (' + instance.status + ')';
                    if (instance.id === currentInstance || instance.name === currentInstance) {
                        option.selected = true;
                    }
                    selector.appendChild(option);
                });
                
                // 如果当前实例不在前10个列表中，但存在于完整列表中，添加它
                if (currentInstance && !displayInstances.some(c => c.id === currentInstance || c.name === currentInstance)) {
                    const fullMatch = instances.find(c => c.id === currentInstance || c.name === currentInstance);
                    if (fullMatch) {
                        const option = document.createElement('option');
                        option.value = fullMatch.id;
                        option.textContent = fullMatch.name + ' (' + fullMatch.status + ')';
                        option.selected = true;
                        selector.appendChild(option);
                    } else {
                        // 当前实例不在完整列表中，添加自定义实例
                        const option = document.createElement('option');
                        option.value = currentInstance;
                        option.textContent = currentInstance;
                        option.selected = true;
                        selector.appendChild(option);
                    }
                }
                
                // 添加分隔线和手动输入选项
                const separator = document.createElement('option');
                separator.disabled = true;
                separator.textContent = '──────────────';
                selector.appendChild(separator);
                
                const manualOption = document.createElement('option');
                manualOption.value = '__manual_input__';
                manualOption.textContent = '✏️ 手动输入实例ID...';
                selector.appendChild(manualOption);
            } catch (error) {
                console.error('Failed to load instances:', error);
                const selector = document.getElementById('container-selector');
                selector.innerHTML = '<option value="">Failed to load</option>';
            }
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
            
            // 如果没有实例参数，弹出输入框要求用户输入
            if (!currentInstance) {
                const instanceId = prompt('请输入实例名称或实例ID:', '');
                if (instanceId && instanceId.trim()) {
                    // 用户输入了实例ID，重定向到带有该参数的URL
                    params.set('instance', instanceId.trim());
                    window.location.search = params.toString();
                    return; // 停止后续初始化，等待页面重新加载
                } else {
                    // 用户取消或没有输入，显示错误信息
                    document.getElementById('terminal').innerHTML = 
                        '<div style="color: #f44336; padding: 20px; text-align: center;">' +
                        '<h2>⚠️ 未指定实例</h2>' +
                        '<p>请刷新页面并输入实例名称或实例ID</p>' +
                        '</div>';
                    document.getElementById('status-text').textContent = 'No instance specified';
                    return; // 停止后续初始化
                }
            }
            
            loadInstances();
            
            // 实例选择器事件
            document.getElementById('container-selector').addEventListener('change', (e) => {
                const selectedValue = e.target.value;
                
                // 如果选择了手动输入选项
                if (selectedValue === '__manual_input__') {
                    const instanceId = prompt('请输入实例名称或实例ID:', '');
                    if (instanceId && instanceId.trim()) {
                        // 用户输入了实例ID，切换到该实例
                        switchInstance(instanceId.trim());
                    } else {
                        // 用户取消或没有输入，恢复到当前选中的实例
                        const params = new URLSearchParams(window.location.search);
                        const currentInstance = params.get('instance');
                        e.target.value = currentInstance || '';
                    }
                } else {
                    // 正常切换实例
                    switchInstance(selectedValue);
                }
            });
            
            // 刷新按钮事件
            document.getElementById('refresh-btn').addEventListener('click', () => {
                loadInstances();
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
            const wsUrl = protocol + '//' + window.location.host + '/terminal/ws' + window.location.search;
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
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// func main() {
// 	port := flag.Int("port", 8080, "HTTP server port")
// 	command := flag.String("command", "/bin/bash", "Default command to execute")
// 	tty := flag.Bool("tty", true, "Allocate a pseudo-TTY")
// 	rows := flag.Int("rows", 24, "Default terminal rows")
// 	cols := flag.Int("cols", 80, "Default terminal columns")
// 	flag.Parse()

// 	defaultCommand = []string{*command}
// 	defaultTTY = *tty
// 	defaultRows = int32(*rows)
// 	defaultCols = int32(*cols)

// 	http.HandleFunc("/", HandleIndex)
// 	http.HandleFunc("/ws", HandleWebSocket)
// 	http.HandleFunc("/api/instances", HandleInstances)
// 	http.Handle("/static/", http.FileServer(http.FS(StaticFiles)))
// 	addr := fmt.Sprintf(":%d", *port)
// 	if err := http.ListenAndServe(addr, nil); err != nil {
// 		log.Fatal("ListenAndServe error: ", err)
// 	}
// }
