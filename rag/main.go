package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/cloudwego/eino-ext/components/model/ark"
)

func main() {
	ctx := context.Background()

	// =============== 1. 初始化 Ark Chat 模型 ===============
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

	// =============== 2. 准备知识库并构建简单向量索引 ===============
	docs := []string{
		"Eino 是字节跳动开源的通用 AI 应用开发框架。",
		"Eino 的 Stream 模式是通过 HTTP SSE（Server-Sent Events） 实现的，模型生成的每个 token 会被实时推送到客户端。",
		"Eino 提供了 Memory（记忆）、RAG（检索增强）、Tool（工具调用）等模块，帮助开发者快速构建智能体。",
	}

	index := buildKnowledgeIndex(docs)
	fmt.Printf("✅ 已建立 %d 条知识的索引\n", len(index))

	// =============== 3. 提问（检索 + 生成） ===============
	query := "Eino 的 Stream 是怎么实现的？"
	contextDocs := retrieveTopK(index, query, 2)
	contextText := formatContext(contextDocs)

	messages := []*schema.Message{
		schema.SystemMessage("你是一个AI助手。请结合提供的知识回答用户问题，如果知识不足以回答，请直接说明不知道。"),
		schema.SystemMessage("检索到的相关知识：\n" + contextText),
		schema.UserMessage(query),
	}

	resp, err := chat.Generate(ctx, messages)
	if err != nil {
		log.Fatalf("生成回答失败: %v", err)
	}

	fmt.Println("\n🤖 问题:", query)
	fmt.Println("💬 模型回答:", resp.Content)
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
		return "（未检索到相关知识）"
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
