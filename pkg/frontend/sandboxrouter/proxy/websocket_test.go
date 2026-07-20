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

package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const websocketTestTimeout = 5 * time.Second

func writeAndFlush(t *testing.T, brw *bufio.ReadWriter, value string) {
	t.Helper()
	if _, err := brw.WriteString(value); err != nil {
		t.Fatalf("write websocket test data: %v", err)
	}
	if err := brw.Flush(); err != nil {
		t.Fatalf("flush websocket test data: %v", err)
	}
}

func serveUpgradeEcho(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "no hijack support", http.StatusInternalServerError)
		return
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	writeAndFlush(t, brw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	for {
		line, err := brw.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				t.Logf("read websocket line: %v", err)
			}
			return
		}
		writeAndFlush(t, brw, line)
	}
}

// TestProxyWebSocketUpgrade verifies the proxy tunnels an HTTP Upgrade
// (WebSocket) handshake through to the backend and streams bytes
// bidirectionally. It uses a raw upgrade rather than a WebSocket library so the
// test stays dependency-free; httputil.ReverseProxy handles the upgrade.
func TestProxyWebSocketUpgrade(t *testing.T) {
	// Upstream hijacks on Upgrade, replies 101, then echoes newline-delimited lines.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveUpgradeEcho(t, w, r)
	}))
	defer upstream.Close()

	ps := httptest.NewServer(New(fakeResolver{target: targetTo(t, upstream.URL)}))
	defer ps.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(ps.URL, "http://"))
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(websocketTestTimeout)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	fmt.Fprint(conn, "GET /inst-a/8080/ws HTTP/1.1\r\nHost: router\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil || !strings.Contains(statusLine, "101") {
		t.Fatalf("expected 101 Switching Protocols, got %q (err %v)", statusLine, err)
	}
	// Drain response headers up to the blank line.
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read headers: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}

	// Send a frame and expect it echoed back through the tunnel.
	fmt.Fprint(conn, "hello\n")
	echo, err := br.ReadString('\n')
	if err != nil || echo != "hello\n" {
		t.Fatalf("echo = %q (err %v), want %q", echo, err, "hello\n")
	}
}
