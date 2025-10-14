package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
)

func getWeather(city string) string {
	city = strings.TrimSpace(city)
	switch city {
	case "北京":
		return "北京今天多云，气温 18~25℃。"
	case "上海":
		return "上海今天小雨，气温 20~27℃。"
	default:
		return fmt.Sprintf("%s 今天晴，气温 22~28℃。", city)
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

	// 1) 初始化 Ark ChatModel
	chat, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   modelID,
	})
	if err != nil {
		log.Fatalf("初始化模型失败: %v", err)
	}

	// 2) 定义工具（新版：schema.ToolInfo + ParameterInfo）
	toolDef := &schema.ToolInfo{
		Name: "getWeather",
		Desc: "根据城市名称获取天气",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"city": {
				Type:     schema.String,
				Desc:     "城市名称",
				Required: true,
			},
		}),
	}

	// 3) 将工具绑定到模型实例（注意：不是 ark.WithTools）
	toolChat, err := chat.WithTools([]*schema.ToolInfo{toolDef})
	if err != nil {
		log.Fatalf("绑定工具失败: %v", err)
	}

	// 4) 用户输入
	user := schema.UserMessage("请告诉我今天北京的天气。")

	// 5) 让模型先决定是否调用工具
	resp, err := toolChat.Generate(ctx, []*schema.Message{
		schema.SystemMessage("你是一个智能助手，必要时可以调用工具获取信息。"),
		user,
	})
	if err != nil {
		log.Fatalf("调用模型失败: %v", err)
	}

	// 6) 检查并处理 Tool 调用
	if len(resp.ToolCalls) == 0 {
		fmt.Println("模型直接回答：", resp.Content)
		return
	}

	for _, call := range resp.ToolCalls {
		fmt.Printf("🧩 模型请求调用工具：%s，参数：%s\n", call.Function.Name, call.Function.Arguments)

		switch call.Function.Name {
		case "getWeather":
			var args struct {
				City string `json:"city"`
			}
			_ = json.Unmarshal([]byte(call.Function.Arguments), &args)

			// 执行你本地的工具函数
			weather := getWeather(args.City)
			fmt.Println("🌤 工具返回结果：", weather)

			// 7) 把工具结果再喂回模型（新版：ToolMessage + WithToolName）
			final, err := toolChat.Generate(ctx, []*schema.Message{
				schema.SystemMessage("你是智能助手"),
				user,
				schema.ToolMessage(weather, call.ID, schema.WithToolName(call.Function.Name)),
			})
			if err != nil {
				log.Fatalf("生成最终回答失败: %v", err)
			}
			fmt.Println("💬 最终回答：", final.Content)

		default:
			fmt.Println("未知工具：", call.Function.Name)
		}
	}
}
