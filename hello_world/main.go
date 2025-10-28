package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
)

func main() {
	ctx := context.Background()

	apiKey := os.Getenv("ARK_API_KEY")
	baseURL := os.Getenv("ARK_BASE_URL")
	modelID := os.Getenv("ARK_MODEL")
	if apiKey == "" || baseURL == "" || modelID == "" {
		log.Fatal("请先设置 ARK_API_KEY / ARK_BASE_URL / ARK_MODEL 环境变量")
	}

	// 初始化 Ark ChatModel（走 Ark 的 OpenAI 兼容服务）
	chatModel, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		BaseURL: baseURL, // 例如: https://ark.cn-beijing.volces.com/api/v3
		APIKey:  apiKey,  // 等同于 curl 的 Authorization: Bearer <key>
		Model:   modelID, // deepseek-v3-1-terminus 或 ep-xxxx
	})
	if err != nil {
		log.Fatalf("初始化 Ark ChatModel 失败: %v", err)
	}

	// 与你的 curl 一致的消息
	msgs := []*schema.Message{
		schema.SystemMessage("你是人工智能助手."),
		schema.UserMessage("你好"),
	}

	// 普通（非流式）生成
	resp, err := chatModel.Generate(ctx, msgs)
	if err != nil {
		log.Fatalf("调用模型失败: %v", err)
	}
	fmt.Println("模型返回：", resp.Content)

}
