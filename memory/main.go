package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
)

// 轻量内存实现
type Memory struct {
	mu       sync.Mutex
	sessions map[string][]*schema.Message
}

func NewMemory() *Memory {
	return &Memory{sessions: make(map[string][]*schema.Message)}
}

func (m *Memory) Get(session string) []*schema.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[session]
}

func (m *Memory) Add(session string, msg *schema.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session] = append(m.sessions[session], msg)
}

// 限制上下文条数，避免过长
func (m *Memory) Trim(session string, max int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs := m.sessions[session]
	if len(msgs) > max {
		m.sessions[session] = msgs[len(msgs)-max:]
	}
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

	mem := NewMemory()

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("多轮对话 Demo 已启动。输入 `/a` 切换到会话A，`/b` 切换到会话B，`/exit` 退出。")

	session := "A"

	for {
		fmt.Printf("[%s] 你：", session)
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if text == "/exit" {
			break
		}
		if strings.HasPrefix(text, "/a") {
			session = "A"
			fmt.Println("👉 已切换到会话 A")
			continue
		}
		if strings.HasPrefix(text, "/b") {
			session = "B"
			fmt.Println("👉 已切换到会话 B")
			continue
		}

		// 将用户输入加入记忆
		mem.Add(session, schema.UserMessage(text))
		mem.Trim(session, 10) // 限制最近10轮

		// 获取上下文
		msgs := mem.Get(session)

		// 调用模型
		resp, err := chatModel.Generate(ctx, msgs)
		if err != nil {
			log.Println("调用模型失败:", err)
			continue
		}

		fmt.Printf("[%s] AI：%s\n", session, resp.Content)

		// 加入AI回复
		mem.Add(session, schema.AssistantMessage(resp.Content, nil))
	}
}
