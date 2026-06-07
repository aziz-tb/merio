package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	openaiopt "github.com/openai/openai-go/option"
)

func main() {
	key := "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"
	if key == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}

	client := openai.NewClient(
		openaiopt.WithAPIKey(key),
		openaiopt.WithMaxRetries(0),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	started := time.Now()
	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel("gpt-4o-mini"),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Say hello in 5 words"),
		},
		MaxTokens: openai.Int(50),
	})
	if err != nil {
		log.Fatalf("openai smoke failed: %v", err)
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		log.Fatalf("openai smoke failed: empty content")
	}

	fmt.Printf(
		"openai ok: id=%s prompt_tokens=%d completion_tokens=%d cached_tokens=%d latency_ms=%d\n",
		resp.ID,
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
		resp.Usage.PromptTokensDetails.CachedTokens,
		time.Since(started).Milliseconds(),
	)
}
