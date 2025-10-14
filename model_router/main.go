package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
)

func main() {
	ctx := context.Background()

	apiKey := os.Getenv("ARK_API_KEY")
	baseURL := os.Getenv("ARK_BASE_URL")
	if apiKey == "" || baseURL == "" {
		log.Fatal("请先设置 ARK_API_KEY 与 ARK_BASE_URL 环境变量")
	}

	modelIDs := map[string]string{
		"default": firstNonEmpty(os.Getenv("ARK_MODEL_DEFAULT"), os.Getenv("ARK_MODEL")),
		"fast":    os.Getenv("ARK_MODEL_FAST"),
		"logic":   os.Getenv("ARK_MODEL_LOGIC"),
	}
	if modelIDs["default"] == "" {
		log.Fatal("请至少配置 ARK_MODEL 或 ARK_MODEL_DEFAULT")
	}

	models := make(map[string]*ark.ChatModel)
	for name, modelID := range modelIDs {
		if modelID == "" {
			continue
		}
		m, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
			BaseURL: baseURL,
			APIKey:  apiKey,
			Model:   modelID,
		})
		if err != nil {
			log.Fatalf("初始化模型 %s 失败: %v", name, err)
		}
		models[name] = m
		fmt.Printf("✅ 模型实例化成功：%s -> %s\n", name, modelID)
	}

	selectModel := func(query string) string {
		lower := strings.ToLower(query)
		switch {
		case strings.Contains(lower, "代码"), strings.Contains(lower, "算法"):
			if models["logic"] != nil {
				return "logic"
			}
		case len([]rune(query)) > 200:
			if models["fast"] != nil {
				return "fast"
			}
		}
		return "default"
	}

	userInput := "请帮我写一个快速排序的 Go 实现"
	modelName := selectModel(userInput)
	fmt.Printf("🧭 已路由到模型：%s\n", modelName)

	selectedModel := models[modelName]
	if selectedModel == nil {
		log.Fatalf("模型 %s 未初始化", modelName)
	}

	messages := []*schema.Message{
		schema.SystemMessage("你是一个智能助手，请详细回答用户的问题。"),
		schema.UserMessage(userInput),
	}

	resp, err := selectedModel.Generate(ctx, messages)
	if err != nil {
		log.Fatalf("模型生成失败: %v", err)
	}

	fmt.Println("🤖 模型回答：", resp.Content)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
