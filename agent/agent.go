// Package agent implements the Fix Fast agentic loop using the IONOS AI Model Hub.
// Architecture based on: https://engineering.fb.com/2021/02/17/developer-tools/fix-fast/
//
// The agent orchestrates four tools in sequence:
//  1. detect_regression  — classify the bug type and severity
//  2. triage_issue       — calculate CPD score and priority (P0-P3)
//  3. attribute_to_owner — route to the right component/team
//  4. generate_fix_plan  — produce an actionable "Get Clean, Stay Clean" plan
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	baseURL      = "https://openai.inference.de-txl.ionos.com/v1/chat/completions"
	defaultModel = "meta-llama/Llama-3.3-70B-Instruct"
	apiKeyEnvVar = "IONOS_API_KEY"
)

const systemPrompt = `You are the Fix Fast agent, inspired by Facebook's regression detection system.
Your mission: analyze bugs and regressions through the Fix Fast framework, then produce an actionable report.

The Fix Fast framework has four principles:
1. SHIFT LEFT   — Detect problems as early as possible (IDE > local_test > CI > code_review > staging > production).
                  A production bug costs 100x more than one caught in the IDE.
2. SIGNAL QUALITY — Focus on meaningful, actionable signals. Reduce noise.
3. FASTER ATTRIBUTION — Route issues to the right owner fast using the multisect principle.
4. GET CLEAN, STAY CLEAN — Fix the root cause AND add safeguards to prevent recurrence.

You MUST use ALL FOUR tools in order for every analysis:
  Step 1: detect_regression  — identify regression type and severity
  Step 2: triage_issue       — calculate CPD score, determine P0/P1/P2/P3 priority
  Step 3: attribute_to_owner — find the highest-confidence owner/component
  Step 4: generate_fix_plan  — produce the complete fix and prevention plan

After all four tools have run, synthesize a final report in this structure:
## Fix Fast Analysis Report

### Detection
[regression type, severity, confidence]

### Triage (CPD Score)
[CPD score, priority, cost rationale]

### Attribution
[component owner, confidence, signals]

### Fix Plan
[immediate actions, fix steps, estimated effort]

### Shift Left Recommendations
[how to catch this class of bug earlier next time]

### Prevention
[measures to prevent recurrence]

Be direct, concrete, and actionable. Engineers need to act fast.`

// chatMessage represents a single message in the conversation.
type chatMessage struct {
	Role       string     `json:"role"`
	Content    *string    `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Tools     []ionosTool   `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Agent wraps the HTTP client and tool definitions.
type Agent struct {
	apiKey string
	model  string
	http   *http.Client
	tools  []toolDef
}

// New creates a new Fix Fast agent. Reads IONOS_API_KEY from the environment.
func New() *Agent {
	model := defaultModel
	if m := os.Getenv("IONOS_MODEL"); m != "" {
		model = m
	}
	return &Agent{
		apiKey: os.Getenv(apiKeyEnvVar),
		model:  model,
		http:   &http.Client{},
		tools:  allTools(),
	}
}

// NewWithKey creates a new Fix Fast agent with an explicit API key.
func NewWithKey(apiKey string) *Agent {
	a := New()
	a.apiKey = apiKey
	return a
}

// Run executes the Fix Fast analysis for the given bug report or diff.
// Streams progress to w and returns the final analysis text.
func (a *Agent) Run(ctx context.Context, input string, w io.Writer) (string, error) {
	if a.apiKey == "" {
		return "", fmt.Errorf("%s environment variable is not set", apiKeyEnvVar)
	}

	strPtr := func(s string) *string { return &s }

	messages := []chatMessage{
		{Role: "system", Content: strPtr(systemPrompt)},
		{Role: "user", Content: strPtr(input)},
	}

	fmt.Fprintln(w, "\n--- Fix Fast Agent Running ---")

	var finalText strings.Builder

	// Agentic loop: keep going until the model stops calling tools.
	for {
		resp, err := a.call(ctx, messages)
		if err != nil {
			return "", err
		}

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("empty response from API")
		}

		msg := resp.Choices[0].Message
		finishReason := resp.Choices[0].FinishReason

		// Print any text content immediately.
		if msg.Content != nil && *msg.Content != "" {
			fmt.Fprint(w, *msg.Content)
			finalText.WriteString(*msg.Content)
		}

		// Append assistant turn to history.
		messages = append(messages, msg)

		// No tool calls → model is done.
		if finishReason == "stop" || len(msg.ToolCalls) == 0 {
			fmt.Fprintln(w, "\n\n--- Analysis Complete ---")
			return finalText.String(), nil
		}

		// Execute each tool call and collect results.
		for _, tc := range msg.ToolCalls {
			fmt.Fprintf(w, "\n\n[tool: %s]\n", tc.Function.Name)

			result, toolErr := dispatch(a.tools, tc.Function.Name, tc.Function.Arguments)
			toolCallID := tc.ID

			var content string
			if toolErr != nil {
				fmt.Fprintf(w, "[tool error: %v]\n", toolErr)
				content = fmt.Sprintf("error: %v", toolErr)
			} else {
				// Pretty-print for readability.
				var pretty interface{}
				if jsonErr := json.Unmarshal([]byte(result), &pretty); jsonErr == nil {
					prettyBytes, _ := json.MarshalIndent(pretty, "", "  ")
					fmt.Fprintf(w, "%s\n", string(prettyBytes))
				}
				content = result
			}

			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallID: toolCallID,
				Content:    &content,
			})
		}
	}
}

// call sends a single chat completions request to the IONOS Model Hub.
func (a *Agent) call(ctx context.Context, messages []chatMessage) (*chatResponse, error) {
	req := chatRequest{
		Model:     a.model,
		Messages:  messages,
		Tools:     toolParams(a.tools),
		MaxTokens: 8192,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := a.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("decode response (status %d): %w\nbody: %s", httpResp.StatusCode, err, respBody)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("API error (%s): %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", httpResp.StatusCode, respBody)
	}

	return &chatResp, nil
}
