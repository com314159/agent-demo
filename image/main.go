package main

import (
	"context"
	"encoding/base64"
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

	chat, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   modelID,
	})
	if err != nil {
		log.Fatalf("初始化 Ark ChatModel 失败: %v", err)
	}

	// ========= 使用本地图片 =========
	imagePath := "/Users/bytedance/Documents/codework/openSource/eino/image/image1.png"
	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		log.Fatalf("读取本地图片失败: %v", err)
	}

	userMsg := schema.UserMessage("")
	userMsg.MultiContent = []schema.ChatMessagePart{
		{
			Type: schema.ChatMessagePartTypeText,
			Text: "请评估该仪表盘的对齐、留白、层级与配色、字体颜色对比度对可阅读性的影响，并给出3条可执行改进建议。",
		},
		{
			Type: schema.ChatMessagePartTypeImageURL,
			ImageURL: &schema.ChatMessageImageURL{
				URL:      fmt.Sprintf("data:image/png;base64,%s", base64.StdEncoding.EncodeToString(imageBytes)),
				MIMEType: "image/png",
				Detail:   schema.ImageURLDetailAuto,
			},
		},
	}

	msgs := []*schema.Message{
		schema.SystemMessage("你是一名数据可视化与UI规范专家，请基于图片判断布局美观性并给出可执行建议。"),
		userMsg,
	}

	resp, err := chat.Generate(ctx, msgs)
	if err != nil {
		log.Fatalf("生成失败: %v", err)
	}
	fmt.Println("🖼️ 本地文件模式结果：\n", resp.Content)
}
