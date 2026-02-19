// Package agent implements the Fix Fast agentic loop using Claude claude-opus-4-6.
// Architecture based on: https://engineering.fb.com/2021/02/17/developer-tools/fix-fast/
//
// The agent orchestrates four tools in sequence:
//  1. detect_regression  — classify the bug type and severity
//  2. triage_issue       — calculate CPD score and priority (P0-P3)
//  3. attribute_to_owner — route to the right component/team
//  4. generate_fix_plan  — produce an actionable "Get Clean, Stay Clean" plan
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const model = "claude-opus-4-6"

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

// Agent wraps the Claude client and tool definitions.
type Agent struct {
	client anthropic.Client
	tools  []toolDef
}

// New creates a new Fix Fast agent. It reads ANTHROPIC_API_KEY from the environment.
func New() *Agent {
	return &Agent{
		client: anthropic.NewClient(),
		tools:  allTools(),
	}
}

// NewWithKey creates a new Fix Fast agent with an explicit API key.
func NewWithKey(apiKey string) *Agent {
	return &Agent{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		tools:  allTools(),
	}
}

// Run executes the Fix Fast analysis for the given bug report or diff.
// Streams progress to w and returns the final analysis text.
func (a *Agent) Run(ctx context.Context, input string, w io.Writer) (string, error) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(input)),
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 8192,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Tools:    toolParams(a.tools),
		Thinking: anthropic.ThinkingConfigParamOfEnabled(4096),
	}

	fmt.Fprintln(w, "\n--- Fix Fast Agent Running ---")

	var finalText strings.Builder

	// Agentic loop: keep going until Claude stops calling tools.
	for {
		message, err := a.client.Messages.New(ctx, params)
		if err != nil {
			return "", fmt.Errorf("API call failed: %w", err)
		}

		// Print any text content blocks immediately.
		for _, block := range message.Content {
			if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
				fmt.Fprint(w, tb.Text)
				finalText.WriteString(tb.Text)
			}
		}

		// Append the assistant turn to history.
		messages = append(messages, message.ToParam())

		// Collect tool call results.
		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range message.Content {
			toolUse, ok := block.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}

			fmt.Fprintf(w, "\n\n[tool: %s]\n", toolUse.Name)

			inputJSON, err := json.Marshal(toolUse.Input)
			if err != nil {
				toolResults = append(toolResults,
					anthropic.NewToolResultBlock(toolUse.ID, "error marshaling input: "+err.Error(), true))
				continue
			}

			result, toolErr := dispatch(a.tools, toolUse.Name, string(inputJSON))
			if toolErr != nil {
				fmt.Fprintf(w, "[tool error: %v]\n", toolErr)
				toolResults = append(toolResults,
					anthropic.NewToolResultBlock(toolUse.ID, toolErr.Error(), true))
			} else {
				// Pretty-print tool result for readability.
				var pretty interface{}
				if jsonErr := json.Unmarshal([]byte(result), &pretty); jsonErr == nil {
					prettyBytes, _ := json.MarshalIndent(pretty, "", "  ")
					fmt.Fprintf(w, "%s\n", string(prettyBytes))
				}
				toolResults = append(toolResults,
					anthropic.NewToolResultBlock(toolUse.ID, result, false))
			}
		}

		// No tool calls → Claude is done.
		if len(toolResults) == 0 {
			fmt.Fprintln(w, "\n\n--- Analysis Complete ---")
			return finalText.String(), nil
		}

		// Feed results back for the next turn.
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
		params.Messages = messages
	}
}
