package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
	ID      any           `json:"id,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	http.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)

		var req JSONRPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, nil, -32700, "parse error")
			return
		}
		switch req.Method {
		case "getWeather":
			var p struct {
				City string `json:"city"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil {
				writeErr(w, req.ID, -32602, "invalid params")
				return
			}
			city := strings.TrimSpace(p.City)
			var ans string
			switch city {
			case "北京":
				ans = "北京今天多云，18~25℃。"
			case "上海":
				ans = "上海今天小雨，20~27℃。"
			default:
				ans = city + " 今天天气晴朗，22~28℃。"
			}
			writeOK(w, req.ID, map[string]any{
				"content": ans,
			})

		case "readFile":
			var p struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil {
				writeErr(w, req.ID, -32602, "invalid params")
				return
			}
			data, err := os.ReadFile(p.Path)
			if err != nil {
				writeErr(w, req.ID, -32000, "read file error: "+err.Error())
				return
			}
			if len(data) > 2000 {
				data = data[:2000] // 防止过大
			}
			writeOK(w, req.ID, map[string]any{
				"content": string(data),
			})

		default:
			writeErr(w, req.ID, -32601, "method not found")
		}
	})

	log.Println("MCP-like JSON-RPC server on http://127.0.0.1:8089/rpc")
	log.Fatal(http.ListenAndServe(":8089", nil))
}

func writeOK(w http.ResponseWriter, id any, result any) {
	resp := JSONRPCResponse{JSONRPC: "2.0", Result: result, ID: id}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeErr(w http.ResponseWriter, id any, code int, msg string) {
	resp := JSONRPCResponse{JSONRPC: "2.0", Error: &JSONRPCError{Code: code, Message: msg}, ID: id}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
