package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	openaiopt "github.com/openai/openai-go/option"
)

// Foundry Anthropic constants — locked by experimentation against the
// real Azure deployment. Do not change without verifying first.
const (
	foundryAnthropicVersion = "2023-06-01"
	foundryAnthropicAuth    = "x-api-key"
)

func main() {
	key := mustEnv("AZURE_API_KEY")
	endpoint := strings.TrimRight(mustEnv("AZURE_FOUNDRY_ENDPOINT"), "/")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	runOpenAI(ctx, key, endpoint+"/openai/v1")
	runAnthropic(ctx, key, endpoint+"/anthropic")
}

// runAnthropic talks to Foundry's Anthropic surface via direct HTTP.
//
// The Anthropic Go SDK explicitly does NOT support Foundry (Anthropic's
// own docs list only C#, Java, PHP, Python, TypeScript). The wire format
// on /anthropic matches the standard Anthropic Messages API, so direct
// HTTP is the supported path for Go.
func runAnthropic(ctx context.Context, key, baseURL string) {
	deployment := envOr("AZURE_ANTHROPIC_DEPLOYMENT", "claude-haiku-4-5")

	body, err := json.Marshal(map[string]any{
		"model":      deployment,
		"max_tokens": 200,
		"messages": []map[string]any{
			{"role": "user", "content": "Say hello in 5 words"},
		},
	})
	if err != nil {
		log.Fatalf("azure anthropic smoke: marshal failed: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		log.Fatalf("azure anthropic smoke: build request failed: %v", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set(foundryAnthropicAuth, key)
	req.Header.Set("anthropic-version", foundryAnthropicVersion)

	started := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("azure anthropic smoke: HTTP error: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("azure anthropic smoke: read body failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("azure anthropic smoke: status=%d body=%s", resp.StatusCode, string(raw))
	}

	var parsed struct {
		ID         string `json:"id"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		log.Fatalf("azure anthropic smoke: parse response failed: %v body=%s", err, string(raw))
	}
	if len(parsed.Content) == 0 {
		log.Fatalf("azure anthropic smoke: empty content (stop_reason=%s)", parsed.StopReason)
	}

	var preview string
	for _, c := range parsed.Content {
		if c.Type == "text" {
			preview = c.Text
			break
		}
	}
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}

	fmt.Printf(
		"azure anthropic ok: id=%s model=%s stop_reason=%s input=%d output=%d cache_created=%d cache_read=%d latency_ms=%d preview=%q\n",
		parsed.ID,
		parsed.Model,
		parsed.StopReason,
		parsed.Usage.InputTokens,
		parsed.Usage.OutputTokens,
		parsed.Usage.CacheCreationInputTokens,
		parsed.Usage.CacheReadInputTokens,
		time.Since(started).Milliseconds(),
		preview,
	)
}

// runOpenAI talks to Foundry's OpenAI surface via the official OpenAI
// Go SDK with the Foundry base URL. Auth header is api-key (Azure
// convention), not Authorization Bearer (OpenAI default).
func runOpenAI(ctx context.Context, key, baseURL string) {
	deployment := envOr("AZURE_OPENAI_DEPLOYMENT", "o4-mini")

	client := openai.NewClient(
		openaiopt.WithBaseURL(baseURL),
		openaiopt.WithAPIKey(key),
		openaiopt.WithHeaderDel("authorization"),
		openaiopt.WithHeader("api-key", key),
		openaiopt.WithMaxRetries(0),
	)

	started := time.Now()
	// o4-mini is a reasoning model: max_completion_tokens is split
	// between internal reasoning and visible output. 2000 is safe for
	// trivial prompts; production should run higher.
	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(deployment),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Say hello in 5 words"),
		},
		MaxCompletionTokens: openai.Int(2000),
	})
	if err != nil {
		log.Fatalf("azure openai smoke failed: %v", err)
	}
	if len(resp.Choices) == 0 {
		log.Fatalf("azure openai smoke failed: no choices in response")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		log.Fatalf(
			"azure openai returned empty content (finish_reason=%s reasoning_tokens=%d total_completion=%d) — likely budget exhausted by reasoning",
			resp.Choices[0].FinishReason,
			resp.Usage.CompletionTokensDetails.ReasoningTokens,
			resp.Usage.CompletionTokens,
		)
	}

	preview := content
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}

	fmt.Printf(
		"azure openai ok: id=%s finish_reason=%s prompt=%d completion=%d reasoning=%d cached=%d latency_ms=%d preview=%q\n",
		resp.ID,
		resp.Choices[0].FinishReason,
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
		resp.Usage.CompletionTokensDetails.ReasoningTokens,
		resp.Usage.PromptTokensDetails.CachedTokens,
		time.Since(started).Milliseconds(),
		preview,
	)
}

func mustEnv(name string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		log.Fatalf("%s is required", name)
	}
	return v
}

func envOr(name, fallback string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	return v
}
