package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
)

type Task struct {
	Title       string    `json:"title"`
	Priority    string    `json:"priority"` // low/normal/high
	Due         time.Time `json:"due"`      // ISO8601
	Description string    `json:"description,omitempty"`
}

func main() {
	ctx := context.Background()

	apiKey := os.Getenv("ARK_API_KEY")
	baseURL := os.Getenv("ARK_BASE_URL")
	modelID := os.Getenv("ARK_MODEL")
	if apiKey == "" || baseURL == "" || modelID == "" {
		log.Fatal("请先设置 ARK_API_KEY / ARK_BASE_URL / ARK_MODEL 环境变量")
	}

	chatModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   modelID,
	})
	if err != nil {
		log.Fatalf("初始化 Ark ChatModel 失败: %v", err)
	}

	// 要模型输出一个结构化 Task
	system := schema.SystemMessage("你是一个只会输出 JSON 的助手。始终返回严格的 JSON，不要输出解释性文字。")
	user := schema.UserMessage(`
从这段话里抽取一个任务对象，并输出严格 JSON（不要多余文本/代码块）：
"请帮我在下周一上午10点之前修复订单同步的Bug，优先级高，提醒写在描述里。"
JSON 字段：
- title (string)
- priority (low|normal|high)
- due (ISO8601，例如 2025-10-20T10:00:00+08:00)
- description (string，可空)
请只输出JSON！`)

	// 非流式生成
	resp, err := chatModel.Generate(ctx, []*schema.Message{system, user})
	if err != nil {
		log.Fatalf("调用模型失败: %v", err)
	}
	raw := strings.TrimSpace(resp.Content)
	fmt.Println("原始输出：", raw)

	task, err := parseTaskJSON(raw)
	if err != nil {
		// 简单自修复：尝试从 ```json ... ``` 代码块中提取
		if fixed := tryExtractJSON(raw); fixed != "" {
			fmt.Println("尝试自修复 JSON...")
			task, err = parseTaskJSON(fixed)
		}
	}
	if err != nil {
		log.Fatalf("解析失败：%v", err)
	}

	fmt.Printf("解析成功：\n  Title=%s\n  Priority=%s\n  Due=%s\n  Desc=%s\n",
		task.Title, task.Priority, task.Due.Format(time.RFC3339), task.Description)
}

func parseTaskJSON(s string) (*Task, error) {
	var t Task
	if err := json.Unmarshal([]byte(s), &t); err != nil {
		return nil, err
	}
	// 基本校验
	if t.Title == "" || t.Priority == "" || t.Due.IsZero() {
		return nil, errors.New("字段缺失或格式错误")
	}
	// 归一化优先级
	switch strings.ToLower(t.Priority) {
	case "low", "normal", "high":
	default:
		return nil, fmt.Errorf("priority 非法: %s", t.Priority)
	}
	return &t, nil
}

var codeBlockJSON = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

func tryExtractJSON(s string) string {
	m := codeBlockJSON.FindStringSubmatch(s)
	if len(m) == 2 {
		return m[1]
	}
	// 退一步：从首个 { 到最后一个 } 之间截取
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}
