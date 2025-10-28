package main

import (
	"context"
	"fmt"
	"io"
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
		schema.UserMessage("你好,请你帮我写一个400字的作文，适合小朋友看的，关于季节的"),
	}

	// 流式生成
	stream, err := chatModel.Stream(ctx, msgs)
	if err != nil {
		log.Fatalf("开启流式调用失败: %v", err)
	}
	defer stream.Close()

	fmt.Println("模型流式返回：")
	chunks := make([]*schema.Message, 0)
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("流式接收失败: %v", err)
		}
		chunks = append(chunks, chunk)
		fmt.Print(chunk.Content)
	}

	full, err := schema.ConcatMessages(chunks)
	if err != nil {
		log.Fatalf("拼接流式消息失败: %v", err)
	}
	fmt.Printf("\n模型完整返回：%s\n", full.Content)

}
