package sandbox

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/frontend/common/util"
)

func TestStreamV1HandlerUploadsAndDownloadsFileChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type testStream struct {
		path   string
		mode   string
		offset int64
	}
	files := map[string][]byte{}
	streams := map[string]*testStream{}
	results := map[string]interface{}{}
	var seq int
	util.SetAPIClientLibruntime(&runtimeStub{
		invokeInstance: func(funcMeta api.FunctionMeta, instanceID string, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			require.Equal(t, "sandbox-123", instanceID)
			require.True(t, invokeOpt.BypassDataSystem)

			kwargs := decodeInvokeEnvelope(t, args)
			var result interface{}
			switch funcMeta.FuncName {
			case "sandbox_stream_open":
				path := fmt.Sprint(kwargs["path"])
				mode := fmt.Sprint(kwargs["mode"])
				streamID := fmt.Sprintf("stream-%d", len(streams)+1)
				streams[streamID] = &testStream{path: path, mode: mode}
				if mode == "write" {
					files[path] = nil
				}
				result = map[string]interface{}{
					"stream_id": streamID,
					"type":      "file",
					"mode":      mode,
					"path":      path,
					"error":     nil,
				}
			case "sandbox_stream_send":
				streamID := fmt.Sprint(kwargs["stream_id"])
				st := streams[streamID]
				require.NotNil(t, st)
				require.Equal(t, "write", st.mode)
				chunk, err := streamBytes(kwargs["data"])
				require.NoError(t, err)
				files[st.path] = append(files[st.path], chunk...)
				st.offset += int64(len(chunk))
				result = map[string]interface{}{
					"stream_id":     streamID,
					"offset":        st.offset,
					"bytes_written": int64(len(chunk)),
					"error":         nil,
				}
			case "sandbox_stream_recv":
				streamID := fmt.Sprint(kwargs["stream_id"])
				st := streams[streamID]
				require.NotNil(t, st)
				require.Equal(t, "read", st.mode)
				limit := int(asInt64(kwargs["limit"]))
				data := files[st.path]
				end := int(st.offset) + limit
				if end > len(data) {
					end = len(data)
				}
				chunk := data[int(st.offset):end]
				st.offset = int64(end)
				result = map[string]interface{}{
					"stream_id":  streamID,
					"data":       chunk,
					"bytes_read": int64(len(chunk)),
					"eof":        end >= len(data),
					"error":      nil,
				}
			case "sandbox_stream_close":
				streamID := fmt.Sprint(kwargs["stream_id"])
				st := streams[streamID]
				require.NotNil(t, st)
				delete(streams, streamID)
				result = map[string]interface{}{
					"stream_id": streamID,
					"path":      st.path,
					"error":     nil,
				}
			default:
				t.Fatalf("unexpected method %s", funcMeta.FuncName)
			}

			seq++
			objectID := fmt.Sprintf("object-%d", seq)
			results[objectID] = result
			return objectID, nil
		},
		getAsync: func(objectID string, cb api.GetAsyncCallback) {
			result, ok := results[objectID]
			require.True(t, ok)
			msg, err := encodeMsgpack(result)
			require.NoError(t, err)
			cb(append(make([]byte, 16), msg...), nil)
		},
	})

	router := gin.New()
	router.GET("/api/sandbox/v1/sandboxes/:sandboxID/stream", StreamV1Handler)
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/sandbox/v1/sandboxes/sandbox-123/stream"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	hello := readStreamText(t, conn)
	require.Equal(t, "hello", hello.Type)
	require.Equal(t, streamProtocolVersion, hello.Protocol)

	require.NoError(t, conn.WriteJSON(streamControlFrame{
		ID:   "up-1",
		Type: "file.upload.start",
		Path: "/tmp/ws-stream.bin",
	}))
	ready := readStreamText(t, conn)
	require.Equal(t, "file.upload.ready", ready.Type)
	require.NotEmpty(t, ready.StreamID)

	frame, err := buildStreamBinaryFrame("up-1", []byte("hello "))
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame))
	ack := readStreamText(t, conn)
	require.Equal(t, "file.upload.ack", ack.Type)
	require.Equal(t, int64(6), ack.Offset)

	frame, err = buildStreamBinaryFrame("up-1", []byte("world"))
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame))
	ack = readStreamText(t, conn)
	require.Equal(t, "file.upload.ack", ack.Type)
	require.Equal(t, int64(11), ack.Offset)

	require.NoError(t, conn.WriteJSON(streamControlFrame{ID: "up-1", Type: "file.upload.finish"}))
	done := readStreamText(t, conn)
	require.Equal(t, "file.upload.done", done.Type)
	require.Equal(t, int64(11), done.Bytes)
	require.Equal(t, []byte("hello world"), files["/tmp/ws-stream.bin"])

	require.NoError(t, conn.WriteJSON(streamControlFrame{
		ID:        "down-1",
		Type:      "file.download.start",
		Path:      "/tmp/ws-stream.bin",
		ChunkSize: 5,
	}))
	var downloaded []byte
	for {
		messageType, payload, err := conn.ReadMessage()
		require.NoError(t, err)
		switch messageType {
		case websocket.BinaryMessage:
			opID, chunk, err := parseStreamBinaryFrame(payload)
			require.NoError(t, err)
			require.Equal(t, "down-1", opID)
			downloaded = append(downloaded, chunk...)
		case websocket.TextMessage:
			var resp streamResponseFrame
			require.NoError(t, json.Unmarshal(payload, &resp))
			require.Equal(t, "file.download.done", resp.Type)
			require.NotEmpty(t, resp.StreamID)
			require.Equal(t, int64(11), resp.BytesRead)
			require.Equal(t, []byte("hello world"), downloaded)
			return
		}
	}
}

func readStreamText(t *testing.T, conn *websocket.Conn) streamResponseFrame {
	t.Helper()
	messageType, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	var resp streamResponseFrame
	require.NoError(t, json.Unmarshal(payload, &resp))
	require.NotEqual(t, "error", resp.Type, "stream error: %s", string(payload))
	return resp
}

func decodeInvokeEnvelope(t *testing.T, args []api.Arg) map[string]interface{} {
	t.Helper()
	out := make(map[string]interface{}, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		key, err := decodePackedArg(args[i].Data)
		require.NoError(t, err)
		value, err := decodePackedArg(args[i+1].Data)
		require.NoError(t, err)
		out[fmt.Sprint(key)] = value
	}
	return out
}

func asInt64(value interface{}) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case uint64:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}
