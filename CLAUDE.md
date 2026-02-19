# LaaS Ladybug — Claude Code Project

## What this is

A regression detection and triage CLI powered by Claude claude-opus-4-6 with extended
thinking, inspired by [Facebook's Fix Fast system](https://engineering.fb.com/2021/02/17/developer-tools/fix-fast/).

Feed it a bug report and it runs four tools in sequence — detect → triage → attribute → fix plan —
then synthesises a **Fix Fast Analysis Report**.

## Build & Run

```bash
# Build
go build ./...

# Run (interactive)
export ANTHROPIC_API_KEY=your_key
go run .

# Run with inline input
go run . "NPE in auth/login.go after v2.3 deploy" production

# Pipe input
echo "panic: runtime error: index out of range" | go run .
```

Valid environments: `ide`, `local_test`, `ci`, `code_review`, `staging`, `production`

## Project structure

```
main.go           — CLI entry point (args / pipe / interactive input)
agent/
  agent.go        — Agentic loop, system prompt, Claude client, MessageNewParams
  tools.go        — Tool definitions, JSON schema generation, dispatch
tools/
  detect.go       — detect_regression: keyword-scoring engine
  triage.go       — triage_issue: CPD scoring, P0-P3 priority
  attribute.go    — attribute_to_owner: file-pattern → component routing
  fix.go          — generate_fix_plan: per-regression-type playbooks
```

## Key facts for Claude

- **Model**: `claude-opus-4-6` with extended thinking (`ThinkingConfigParamOfEnabled(4096)`)
- **SDK**: `github.com/anthropics/anthropic-sdk-go v1.25.0`
- **Tools field** expects `[]anthropic.ToolUnionParam` (wrap `ToolParam` in `ToolUnionParam{OfTool: &p}`)
- **Thinking field** uses `anthropic.ThinkingConfigParamUnion` (via `ThinkingConfigParamOfEnabled`)
- **`anthropic.NewClient()`** returns a value (`anthropic.Client`), not a pointer

## Skills

This project has a custom skill for analysing LaaS E2E test failures:

- `.claude/skills/analayse-test-failures/` — `analyze-laas-failures` skill
  Invoke with: `/analyze-laas-failures <namespace> [suite] [--since N]`
