package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      int             `json:"id,omitempty"`
}
type rpcResp struct {
	JSONRPC string `json:"jsonrpc"`
	Result  any    `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	ID int `json:"id,omitempty"`
}

var up = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func main() {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade:", err)
			return
		}
		defer c.Close()
		log.Println("WS connected")

		// 主动推送事件：onTick（模拟 MCP 通知）
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		go func() {
			for t := range ticker.C {
				evt := map[string]any{
					"jsonrpc": "2.0",
					"method":  "onTick",
					"params":  map[string]any{"now": t.Format(time.RFC3339)},
				}
				_ = c.WriteJSON(evt)
			}
		}()

		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}

			var req rpcReq
			if err := json.Unmarshal(data, &req); err != nil {
				continue
			}

			switch req.Method {
			case "getTime":
				resp := rpcResp{JSONRPC: "2.0", ID: req.ID,
					Result: map[string]any{"now": time.Now().Format(time.RFC3339)}}
				_ = c.WriteJSON(resp)

			default:
				resp := rpcResp{JSONRPC: "2.0", ID: req.ID,
					Error: (*struct {
						Code    int    `json:"code"`
						Message string `json:"message"`
					})(&struct {
						Code    int
						Message string
					}{-32601, "method not found"})}
				_ = c.WriteJSON(resp)
			}
		}
	})

	log.Println("WS JSON-RPC server at ws://127.0.0.1:8091/ws")
	log.Fatal(http.ListenAndServe(":8091", nil))
}
