package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
	"github.com/gorilla/websocket"
)

type rpcReq struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      int64       `json:"id"`
}
type rpcResp struct {
	JSONRPC string           `json:"jsonrpc"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	ID int64 `json:"id,omitempty"`
}

type wsClient struct {
	conn   *websocket.Conn
	nextID int64
}

func (c *wsClient) call(method string, params any) (map[string]any, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	req := rpcReq{JSONRPC: "2.0", Method: method, Params: params, ID: id}
	if err := c.conn.WriteJSON(req); err != nil {
		return nil, err
	}

	for {
		var msg map[string]json.RawMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			return nil, err
		}

		// 如果是事件（没有 id），交给上层处理
		if _, ok := msg["id"]; !ok {
			// 放回给上层（Host）自己处理：这里直接打印
			var m struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			_ = json.Unmarshal([]byte(toBytes(msg)), &m)
			log.Printf("<< event %s %s\n", m.Method, string(m.Params))
			continue
		}

		var resp rpcResp
		_ = json.Unmarshal([]byte(toBytes(msg)), &resp)
		if resp.ID != id { // 不是我的响应，继续读（简单处理）
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		var m map[string]any
		if resp.Result != nil {
			_ = json.Unmarshal(*resp.Result, &m)
		}
		return m, nil
	}
}
func toBytes(m map[string]json.RawMessage) []byte { b, _ := json.Marshal(m); return b }

func main() {
	ctx := context.Background()
	apiKey, baseURL, modelID := os.Getenv("ARK_API_KEY"), os.Getenv("ARK_BASE_URL"), os.Getenv("ARK_MODEL")
	if apiKey == "" || baseURL == "" || modelID == "" {
		log.Fatal("缺少 ARK_* 环境变量")
	}

	chat, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{BaseURL: baseURL, APIKey: apiKey, Model: modelID})
	if err != nil {
		log.Fatal(err)
	}

	// 连接到“类 MCP Server”（WebSocket JSON-RPC）
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8091/ws", nil)
	if err != nil {
		log.Fatal("dial ws:", err)
	}
	defer ws.Close()
	client := &wsClient{conn: ws}

	// 定义工具：getTime（通过 WS 调用）
	getTimeTool := &schema.ToolInfo{
		Name: "getTime",
		Desc: "获取当前时间（由 Host 通过 WS 向 MCP Server 调用）",
		// 无参：不给 ParamsOneOf 也行；这里给个空的占位
	}

	toolChat, err := chat.WithTools([]*schema.ToolInfo{getTimeTool})
	if err != nil {
		log.Fatal("WithTools:", err)
	}

	// 1) 常规一问一答：模型若需要时间，会触发 tool call
	user := schema.UserMessage("请告诉我现在的时间，并加上一句问候。必要时请调用工具 getTime。")
	resp, err := toolChat.Generate(ctx, []*schema.Message{
		schema.SystemMessage("你是智能助手。需要当前时间时请调用工具 getTime。"),
		user,
	})
	if err != nil {
		log.Fatal(err)
	}

	if len(resp.ToolCalls) == 0 {
		fmt.Println("模型直接回答：", resp.Content)
	} else {
		for _, call := range resp.ToolCalls {
			if call.Function.Name != "getTime" {
				continue
			}
			// 调 WS JSON-RPC
			res, err := client.call("getTime", map[string]any{})
			if err != nil {
				log.Println("rpc getTime:", err)
				res = map[string]any{"now": "调用失败"}
			}

			// 回灌工具结果
			final, err := toolChat.Generate(ctx, []*schema.Message{
				schema.SystemMessage("你是智能助手"),
				user,
				schema.ToolMessage(mustJSON(res), call.ID, schema.WithToolName(call.Function.Name)),
			})
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("最终回答：", final.Content)
		}
	}

	// 2) 体现“双向事件”：Server 每 5 秒推 onTick，Host 收到后再触发一次模型推理
	go func() {
		for {
			var msg map[string]json.RawMessage
			if err := ws.ReadJSON(&msg); err != nil {
				log.Println("ws read:", err)
				return
			}

			// 事件（没有 id）
			if _, ok := msg["id"]; !ok {
				var m struct {
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				_ = json.Unmarshal([]byte(toBytes(msg)), &m)
				if m.Method == "onTick" {
					var p struct {
						Now string `json:"now"`
					}
					_ = json.Unmarshal(m.Params, &p)
					// 收到事件后触发一轮新的推理（演示 MCP 的双向）
					u := schema.UserMessage("收到 onTick 事件，当前时间是：" + p.Now + "。请转为北京时间告诉我时间。")
					ans, err := chat.Generate(ctx, []*schema.Message{
						schema.SystemMessage("你是简洁的助手，只用一句话回复。"),
						u,
					})
					if err == nil {
						fmt.Println("事件回应：", ans.Content)
					}
				}
			}
		}
	}()

	// 阻塞一会儿，观测 onTick 事件（也可以改成 select {} 持续运行）
	time.Sleep(16 * time.Second)
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
