package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
)

// 一个通用的 Agent：给不同的 system prompt 就是不同角色
type Agent struct {
	name   string
	system string
	model  *ark.ChatModel
}

func (a *Agent) Act(ctx context.Context, history []*schema.Message, userHint string) (string, error) {
	msgs := make([]*schema.Message, 0, len(history)+2)
	msgs = append(msgs, schema.SystemMessage(a.system))
	msgs = append(msgs, history...)
	if strings.TrimSpace(userHint) != "" {
		msgs = append(msgs, schema.UserMessage(userHint))
	}
	resp, err := a.model.Generate(ctx, msgs)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func main() {
	ctx := context.Background()

	apiKey, baseURL, modelID := os.Getenv("ARK_API_KEY"), os.Getenv("ARK_BASE_URL"), os.Getenv("ARK_MODEL")
	if apiKey == "" || baseURL == "" || modelID == "" {
		log.Fatal("请设置 ARK_API_KEY / ARK_BASE_URL / ARK_MODEL")
	}

	chat, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   modelID,
		// 可选：增加超时/重试等
	})
	if err != nil {
		log.Fatal("init model:", err)
	}

	// === 定义三个 Agent 的角色提示 ===
	coordinator := &Agent{
		name: "Coordinator",
		system: `你是协调者。目标：把用户任务拆成两步并收敛到最终答案。
规则：
1) 首轮产出一个“任务计划（简短要点）”，并把子任务分给 Researcher 与 Writer。
2) 每轮读完其它代理的输出后，判断是否“可以给最终答案”；如未完成，则给出“下一步指令”给指定代理。
3) 输出以【To:AgentName】开头指派，最终完成时以【FINAL】开头给出最终答案（简洁明了）。`,
		model: chat,
	}
	researcher := &Agent{
		name: "Researcher",
		system: `你是研究员，擅长列要点、补充事实与提纲。不要长篇输出。
读到协调者给你的任务后：
- 只输出与主题强相关的3~6条要点（必要时包含示例/注意事项）。`,
		model: chat,
	}
	writer := &Agent{
		name: "Writer",
		system: `你是撰稿人，擅长把提纲转成清晰的中文说明（短小精悍）。
读到协调者/研究员的要点后，整理成成型段落，保留技术术语，语言简洁。`,
		model: chat,
	}

	// === 共享黑板（多代理共享的对话历史）===
	blackboard := []*schema.Message{}
	// 用户主题
	topic := "请围绕 Eino 的流式输出（SSE）写一段简洁说明，并给出两条实践建议。"

	// 首轮：由协调者生成计划并指派
	fmt.Println("👤 User:", topic)
	plan, err := coordinator.Act(ctx, blackboard, topic)
	mustOK(err)
	blackboard = append(blackboard, schema.AssistantMessage(fmt.Sprintf("[Coordinator]\n%s", plan), nil))
	fmt.Println("\n🤖 Coordinator:\n", plan)

	// 最多 3 个循环：Co -> (Researcher/Writer) -> Co 决定是否结束
	for turn := 1; turn <= 3; turn++ {
		// 检查是否已经给出FINAL
		if strings.Contains(plan, "【FINAL】") {
			break
		}

		// 从协调者输出里解析指派对象（最简单规则匹配）
		target := parseTarget(plan) // "Researcher" / "Writer" / ""
		if target == "" {
			// 没有明确指派，则让协调者直接收敛
			plan, err = coordinator.Act(ctx, blackboard, "请根据当前信息直接给出【FINAL】答案。")
			mustOK(err)
			blackboard = append(blackboard, schema.AssistantMessage(fmt.Sprintf("[Coordinator]\n%s", plan), nil))
			fmt.Println("\n🤖 Coordinator:\n", plan)
			break
		}

		var out string
		switch target {
		case "Researcher":
			out, err = researcher.Act(ctx, blackboard, "")
		case "Writer":
			out, err = writer.Act(ctx, blackboard, "")
		default:
			out = "（无法识别的目标代理）"
		}
		mustOK(err)
		blackboard = append(blackboard, schema.AssistantMessage(fmt.Sprintf("[%s]\n%s", target, out), nil))
		fmt.Printf("\n🤖 %s:\n %s\n", target, out)

		// 让协调者读完黑板后，决定下一步/终止
		plan, err = coordinator.Act(ctx, blackboard, "请阅读黑板最新内容，若已充分则输出【FINAL】；否则继续用【To:AgentName】派发。")
		mustOK(err)
		blackboard = append(blackboard, schema.AssistantMessage(fmt.Sprintf("[Coordinator]\n%s", plan), nil))
		fmt.Println("\n🤖 Coordinator:\n", plan)

		// 可选：给每轮一个最大片段时间，避免卡死
		time.Sleep(200 * time.Millisecond)
	}

	// 打印最终答案
	for i := len(blackboard) - 1; i >= 0; i-- {
		if strings.Contains(blackboard[i].Content, "【FINAL】") {
			fmt.Println("\n✅ 最终结果：")
			fmt.Println(blackboard[i].Content)
			break
		}
	}
}

func parseTarget(s string) string {
	// 期望格式：【To:Researcher】或【To:Writer】
	if i := strings.Index(s, "【To:"); i >= 0 {
		j := strings.Index(s[i:], "】")
		if j > len("【To:") {
			name := strings.TrimSpace(s[i+len("【To:") : i+j])
			return name
		}
	}
	return ""
}

func mustOK(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
