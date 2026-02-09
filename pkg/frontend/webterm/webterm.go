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
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"frontend/pkg/common/faas_common/grpc/pb/exec_service"
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

// NodeAddressResponse 定义从 master 返回的节点地址响应结构
// TODO: 根据实际接口调整字段
type NodeAddressResponse struct {
	NodeAddr string `json:"node_addr"` // 节点地址，如 "192.168.1.10:8080"
	// 可根据需要添加其他字段，如：
	// Status   string `json:"status"`
	// Message  string `json:"message"`
}

// ContainerInfo 定义容器信息结构
// TODO: 根据实际接口调整字段
type ContainerInfo struct {
	ID     string `json:"id"`     // 容器ID
	Name   string `json:"name"`   // 容器名称
	Status string `json:"status"` // 容器状态，如 "running", "stopped"
	// 可根据需要添加其他字段，如：
	// Image    string `json:"image"`
	// NodeAddr string `json:"node_addr"`
	// Created  string `json:"created"`
}

// ContainerListResponse 定义容器列表响应结构
// TODO: 根据实际接口调整字段
type ContainerListResponse struct {
	Containers []ContainerInfo `json:"containers"`
	// 可根据需要添加其他字段，如：
	// Total int `json:"total"`
}

// queryMaster 通用的 master 查询函数
// apiPath: API 路径，如 "/api/v1/containers" 或 "/api/v1/container/node"
// queryParams: 查询参数映射，如 map[string]string{"container": "xxx"}
// result: 用于接收响应的结构体指针
func queryMaster(apiPath string, queryParams map[string]string, result interface{}) error {
	// 获取 master 地址
	masterAddr := util.NewClient().GetActiveMasterAddr()
	if masterAddr == "" {
		return fmt.Errorf("failed to get master address")
	}

	// 构建查询URL
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

	// 创建 HTTP 请求
	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// TODO: 根据需要添加请求头
	// req.Header.Set("Authorization", "Bearer <token>")
	req.Header.Set("Content-Type", "application/json")

	// 发起 HTTP 请求，设置超时
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to query master: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("master returned error status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

func getExecAddr(container string) (string, error) {
	if container == "" {
		return "", fmt.Errorf("container ID cannot be empty")
	}

	// TODO: 根据实际接口调整 API 路径
	// 示例: /api/v1/container/{container}/node
	// 或: /api/v1/nodes/query
	apiPath := "/api/v1/container/node" // 替换为实际的 API 路径
	queryParams := map[string]string{
		"container": container,
	}

	// 调用通用查询函数
	var response NodeAddressResponse
	if err := queryMaster(apiPath, queryParams, &response); err != nil {
		return "", err
	}

	// TODO: 根据实际响应结构调整字段访问
	if response.NodeAddr == "" {
		return "", fmt.Errorf("node address is empty in response")
	}

	log.Printf("Container %s is on node: %s", container, response.NodeAddr)
	return response.NodeAddr, nil
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	sessionID := uuid.New().String()
	log.Printf("WebSocket client connected, session: %s", sessionID)

	// 从URL参数读取配置
	query := r.URL.Query()
	container := query.Get("container")

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

	// 连接到 executor backend
	addr, err := getExecAddr(container)
	if err != nil {
		log.Printf("Failed to get executor address: %v", err)
		return
	}
	grpcConn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Failed to connect to executor: %v", err)
		return
	}
	defer grpcConn.Close()

	client := exec_service.NewExecServiceClient(grpcConn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.ExecStream(ctx)
	if err != nil {
		log.Printf("Failed to create ExecStream: %v", err)
		return
	}

	session := &wsSession{
		ws:         conn,
		grpcStream: stream,
		sessionID:  sessionID,
		ctx:        ctx,
		cancel:     cancel,
	}

	// 发送启动请求
	log.Printf("Starting: container=%s, command=%v, tty=%v, size=%dx%d",
		container, command, tty, cols, rows)
	err = stream.Send(&exec_service.ExecMessage{
		SessionId: sessionID,
		Payload: &exec_service.ExecMessage_StartRequest{
			StartRequest: &exec_service.ExecStartRequest{
				ContainerId: container,
				Command:     command,
				Tty:         tty,
				Rows:        rows,
				Cols:        cols,
			},
		},
	})
	if err != nil {
		log.Printf("Failed to send start request: %v", err)
		return
	}

	done := make(chan struct{})

	// 从 gRPC 读取输出并发送到 WebSocket
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
				log.Printf("Session %s: gRPC stream closed", sessionID)
				return
			}
			if err != nil {
				log.Printf("Session %s: gRPC recv error: %v", sessionID, err)
				return
			}

			switch payload := msg.Payload.(type) {
			case *exec_service.ExecMessage_OutputData:
				session.mu.Lock()
				err := conn.WriteMessage(websocket.BinaryMessage, payload.OutputData.Data)
				session.mu.Unlock()
				if err != nil {
					log.Printf("WebSocket write error: %v", err)
					return
				}

			case *exec_service.ExecMessage_Status:
				log.Printf("Session %s: status=%v, exit_code=%d, error=%s",
					sessionID, payload.Status.Status, payload.Status.ExitCode, payload.Status.ErrorMessage)

				if payload.Status.Status == exec_service.ExecStatusResponse_EXITED ||
					payload.Status.Status == exec_service.ExecStatusResponse_ERROR {
					// 通知WebSocket客户端进程已退出
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

	// 从 WebSocket 读取输入并发送到 gRPC
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
					log.Printf("WebSocket read error: %v", err)
				}
				return
			}

			switch messageType {
			case websocket.TextMessage:
				// 检查是否是resize消息 (格式: "RESIZE:cols:rows")
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
							log.Printf("gRPC resize error: %v", err)
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
					log.Printf("gRPC send error: %v", err)
					return
				}
			}
		}
	}()

	<-done
	log.Printf("Session %s disconnected", sessionID)
}

// handleInstances 返回容器列表，从 master 查询
func HandleInstances(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// TODO: 根据实际接口调整 API 路径
	// 示例: /api/v1/containers 或 /api/v1/containers/list
	apiPath := "/api/v1/containers" // 替换为实际的 API 路径
	
	// 可以根据需要添加查询参数，如分页、过滤等
	queryParams := map[string]string{}
	// 示例: queryParams["status"] = "running"
	// 示例: queryParams["limit"] = "100"

	// 调用通用查询函数
	var response ContainerListResponse
	if err := queryMaster(apiPath, queryParams, &response); err != nil {
		log.Printf("Failed to query containers from master: %v", err)
		// 查询失败时返回空列表，而不是报错，以便前端可以继续工作
		response.Containers = []ContainerInfo{}
	}

	// 返回容器列表
	if err := json.NewEncoder(w).Encode(response.Containers); err != nil {
		log.Printf("Error encoding containers: %v", err)
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
            <select id="container-selector" title="Select container">
                <option value="">Loading containers...</option>
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
        // 加载容器列表
        async function loadContainers() {
            try {
                const response = await fetch('/api/instances');
                const containers = await response.json();
                const selector = document.getElementById('container-selector');
                
                // 清空选项
                selector.innerHTML = '';
                
                // 获取当前容器（从URL参数）
                const params = new URLSearchParams(window.location.search);
                const currentContainer = params.get('container') || '';
                
                // 只显示前10个容器
                const displayContainers = containers.slice(0, 10);
                
                // 添加容器选项
                displayContainers.forEach(container => {
                    const option = document.createElement('option');
                    option.value = container.id;
                    option.textContent = container.name + ' (' + container.status + ')';
                    if (container.id === currentContainer || container.name === currentContainer) {
                        option.selected = true;
                    }
                    selector.appendChild(option);
                });
                
                // 如果当前容器不在前10个列表中，但存在于完整列表中，添加它
                if (currentContainer && !displayContainers.some(c => c.id === currentContainer || c.name === currentContainer)) {
                    const fullMatch = containers.find(c => c.id === currentContainer || c.name === currentContainer);
                    if (fullMatch) {
                        const option = document.createElement('option');
                        option.value = fullMatch.id;
                        option.textContent = fullMatch.name + ' (' + fullMatch.status + ')';
                        option.selected = true;
                        selector.appendChild(option);
                    } else {
                        // 当前容器不在完整列表中，添加自定义容器
                        const option = document.createElement('option');
                        option.value = currentContainer;
                        option.textContent = currentContainer;
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
                manualOption.textContent = '✏️ 手动输入容器ID...';
                selector.appendChild(manualOption);
            } catch (error) {
                console.error('Failed to load containers:', error);
                const selector = document.getElementById('container-selector');
                selector.innerHTML = '<option value="">Failed to load</option>';
            }
        }
        
        // 切换容器
        function switchContainer(containerId) {
            const params = new URLSearchParams(window.location.search);
            if (containerId) {
                params.set('container', containerId);
            } else {
                params.delete('container');
            }
            // 重新加载页面并带上新的容器参数
            window.location.search = params.toString();
        }
        
        // 初始化
        document.addEventListener('DOMContentLoaded', () => {
            // 检查是否有容器参数
            const params = new URLSearchParams(window.location.search);
            const currentContainer = params.get('container');
            
            // 如果没有容器参数，弹出输入框要求用户输入
            if (!currentContainer) {
                const containerId = prompt('请输入容器名称或容器ID:', '');
                if (containerId && containerId.trim()) {
                    // 用户输入了容器ID，重定向到带有该参数的URL
                    params.set('container', containerId.trim());
                    window.location.search = params.toString();
                    return; // 停止后续初始化，等待页面重新加载
                } else {
                    // 用户取消或没有输入，显示错误信息
                    document.getElementById('terminal').innerHTML = 
                        '<div style="color: #f44336; padding: 20px; text-align: center;">' +
                        '<h2>⚠️ 未指定容器</h2>' +
                        '<p>请刷新页面并输入容器名称或容器ID</p>' +
                        '</div>';
                    document.getElementById('status-text').textContent = 'No container specified';
                    return; // 停止后续初始化
                }
            }
            
            loadContainers();
            
            // 容器选择器事件
            document.getElementById('container-selector').addEventListener('change', (e) => {
                const selectedValue = e.target.value;
                
                // 如果选择了手动输入选项
                if (selectedValue === '__manual_input__') {
                    const containerId = prompt('请输入容器名称或容器ID:', '');
                    if (containerId && containerId.trim()) {
                        // 用户输入了容器ID，切换到该容器
                        switchContainer(containerId.trim());
                    } else {
                        // 用户取消或没有输入，恢复到当前选中的容器
                        const params = new URLSearchParams(window.location.search);
                        const currentContainer = params.get('container');
                        e.target.value = currentContainer || '';
                    }
                } else {
                    // 正常切换容器
                    switchContainer(selectedValue);
                }
            });
            
            // 刷新按钮事件
            document.getElementById('refresh-btn').addEventListener('click', () => {
                loadContainers();
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
