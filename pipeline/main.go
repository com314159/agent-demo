package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/cloudwego/eino-ext/components/model/ark"
)

func main() {
	ctx := context.Background()

	apiKey := os.Getenv("ARK_API_KEY")
	baseURL := os.Getenv("ARK_BASE_URL")
	modelID := os.Getenv("ARK_MODEL")
	if apiKey == "" || baseURL == "" || modelID == "" {
		log.Fatal("请设置 ARK_API_KEY / ARK_BASE_URL / ARK_MODEL 环境变量")
	}

	chat, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   modelID,
	})
	if err != nil {
		log.Fatalf("初始化 Ark ChatModel 失败: %v", err)
	}

	docs := []string{
		"Eino 是字节跳动开源的通用 AI 应用开发框架。",
		"Eino 的 Stream 模式是通过 HTTP SSE（Server-Sent Events） 实现的，模型生成的每个 token 会被实时推送到客户端。",
		"Eino 提供了 Memory（记忆）、RAG（检索增强）、Tool（工具调用）等模块，帮助开发者快速构建智能体。",
		"Eino orchestration 能够用图编排的方式拼装节点，使复杂流程可视化、可复用。",
	}

	index := buildKnowledgeIndex(docs)

	graph := compose.NewGraph[string, *schema.Message]()

	err = graph.AddLambdaNode("router", compose.InvokableLambda(func(ctx context.Context, query string) (routePayload, error) {
		route := chooseRoute(query)
		fmt.Printf("🧭 路由选择: %s -> %s\n", query, route)
		return routePayload{Query: query, Route: route}, nil
	}))
	if err != nil {
		log.Fatalf("添加路由节点失败: %v", err)
	}

	err = graph.AddLambdaNode("rag_handler", compose.InvokableLambda(func(ctx context.Context, payload routePayload) (*schema.Message, error) {
		contextDocs := retrieveTopK(index, payload.Query, 2)
		contextText := formatContext(contextDocs)
		if contextText == "" {
			contextText = "（未检索到相关知识）"
		}

		messages := []*schema.Message{
			schema.SystemMessage("你是一个知识助手，请结合提供的知识回答问题，回答时引用知识条目编号。"),
			schema.SystemMessage("相关知识：\n" + contextText),
			schema.UserMessage(payload.Query),
		}
		return chat.Generate(ctx, messages)
	}))
	if err != nil {
		log.Fatalf("添加 RAG 节点失败: %v", err)
	}

	err = graph.AddLambdaNode("chat_handler", compose.InvokableLambda(func(ctx context.Context, payload routePayload) (*schema.Message, error) {
		messages := []*schema.Message{
			schema.SystemMessage("你是一个通用的智能助手，以简洁方式回答用户问题。"),
			schema.UserMessage(payload.Query),
		}
		return chat.Generate(ctx, messages)
	}))
	if err != nil {
		log.Fatalf("添加普通对话节点失败: %v", err)
	}

	err = graph.AddEdge(compose.START, "router")
	if err != nil {
		log.Fatalf("连接 START->router 失败: %v", err)
	}

	err = graph.AddEdge("rag_handler", compose.END)
	if err != nil {
		log.Fatalf("连接 rag_handler->END 失败: %v", err)
	}

	err = graph.AddEdge("chat_handler", compose.END)
	if err != nil {
		log.Fatalf("连接 chat_handler->END 失败: %v", err)
	}

	branch := compose.NewGraphBranch(func(ctx context.Context, payload routePayload) (string, error) {
		switch payload.Route {
		case "rag":
			return "rag_handler", nil
		default:
			return "chat_handler", nil
		}
	}, map[string]bool{
		"rag_handler":  true,
		"chat_handler": true,
	})

	err = graph.AddBranch("router", branch)
	if err != nil {
		log.Fatalf("添加分支失败: %v", err)
	}

	runnable, err := graph.Compile(ctx)
	if err != nil {
		log.Fatalf("编译图失败: %v", err)
	}

	queries := []string{
		"Eino 的编排能力是什么？",
		"今天天气怎么样？",
	}

	for _, q := range queries {
		resp, err := runnable.Invoke(ctx, q)
		if err != nil {
			log.Printf("处理问题失败（%s）: %v\n", q, err)
			continue
		}
		fmt.Printf("\n🤖 问题: %s\n", q)
		fmt.Printf("💡 回答: %s\n", resp.Content)
	}
}

type routePayload struct {
	Query string
	Route string
}

func chooseRoute(query string) string {
	lower := strings.ToLower(query)
	if strings.Contains(lower, "eino") || strings.Contains(lower, "框架") {
		return "rag"
	}
	return "chat"
}

type knowledgeDoc struct {
	Text   string
	Vector map[string]float64
}

func buildKnowledgeIndex(docs []string) []knowledgeDoc {
	index := make([]knowledgeDoc, len(docs))
	for i, doc := range docs {
		index[i] = knowledgeDoc{
			Text:   doc,
			Vector: textToVector(doc),
		}
	}
	return index
}

func retrieveTopK(index []knowledgeDoc, query string, k int) []knowledgeDoc {
	qVec := textToVector(query)
	type scored struct {
		doc   knowledgeDoc
		score float64
	}

	scoredDocs := make([]scored, 0, len(index))
	for _, doc := range index {
		score := cosineSimilarity(qVec, doc.Vector)
		scoredDocs = append(scoredDocs, scored{doc: doc, score: score})
	}

	sort.Slice(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].score > scoredDocs[j].score
	})

	if k > len(scoredDocs) {
		k = len(scoredDocs)
	}

	results := make([]knowledgeDoc, 0, k)
	for i := 0; i < k; i++ {
		if scoredDocs[i].score <= 0 {
			break
		}
		results = append(results, scoredDocs[i].doc)
	}
	return results
}

func formatContext(docs []knowledgeDoc) string {
	if len(docs) == 0 {
		return ""
	}
	var builder strings.Builder
	for i, doc := range docs {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, doc.Text))
	}
	return builder.String()
}

func textToVector(text string) map[string]float64 {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return map[string]float64{}
	}

	vec := make(map[string]float64)
	for _, token := range tokens {
		vec[token]++
	}

	total := float64(len(tokens))
	for key := range vec {
		vec[key] /= total
	}
	return vec
}

func tokenize(text string) []string {
	normalized := strings.ToLower(text)
	normalized = strings.NewReplacer(
		"。", " ",
		"，", " ",
		"、", " ",
		"；", " ",
		"：", " ",
		"！", " ",
		"？", " ",
		".", " ",
		",", " ",
		";", " ",
		":", " ",
		"!", " ",
		"?", " ",
	).Replace(normalized)

	fields := strings.Fields(normalized)
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			result = append(result, f)
		}
	}
	return result
}

func cosineSimilarity(a, b map[string]float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	var dot float64
	var normA float64
	var normB float64

	for key, av := range a {
		normA += av * av
		if bv, ok := b[key]; ok {
			dot += av * bv
		}
	}

	for _, bv := range b {
		normB += bv * bv
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
