package agent

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/emyjamalian/laas-ladybug/tools"
)

// toolDef holds the Anthropic tool param and the Go handler function.
type toolDef struct {
	Param   anthropic.ToolParam
	Handler func(inputJSON string) (string, error)
}

// allTools returns the complete set of Fix Fast tools with their definitions.
func allTools() []toolDef {
	return []toolDef{
		{
			Param: anthropic.ToolParam{
				Name: "detect_regression",
				Description: anthropic.String(
					"Analyzes a bug report, code change, or error message to determine if it is a regression. " +
						"Returns the regression type (null_pointer, performance, crash, memory_leak, logic_error, " +
						"data_corruption, api_breaking_change, security_flaw), severity, affected components, " +
						"and detection confidence. Always call this first.",
				),
				InputSchema: schemaFor(tools.DetectRegressionInput{}),
			},
			Handler: tools.DetectRegression,
		},
		{
			Param: anthropic.ToolParam{
				Name: "triage_issue",
				Description: anthropic.String(
					"Calculates the Cost Per Developer (CPD) score for a regression. " +
						"CPD is Facebook's metric: bugs cost exponentially more the further downstream they are detected. " +
						"Production bugs are 100x more expensive than IDE-caught bugs. " +
						"Returns priority (P0-P3), recommended action, and a 'shift left' target environment. " +
						"Call this after detect_regression.",
				),
				InputSchema: schemaFor(tools.TriageIssueInput{}),
			},
			Handler: tools.TriageIssue,
		},
		{
			Param: anthropic.ToolParam{
				Name: "attribute_to_owner",
				Description: anthropic.String(
					"Attributes the regression to the most likely code component and owner by analyzing " +
						"changed files and the regression description. Uses the 'multisect' principle from " +
						"Fix Fast to route issues to the right team 3x faster. " +
						"Returns suspected owners with confidence scores. " +
						"Call this after triage_issue.",
				),
				InputSchema: schemaFor(tools.AttributeIssueInput{}),
			},
			Handler: tools.AttributeToOwner,
		},
		{
			Param: anthropic.ToolParam{
				Name: "generate_fix_plan",
				Description: anthropic.String(
					"Generates a concrete, step-by-step fix plan for the regression. " +
						"Implements the 'Get Clean, Stay Clean' principle from Fix Fast: immediate mitigation, " +
						"root cause fix, prevention measures, and 'shift left' recommendations to catch this " +
						"class of bug earlier in the development pipeline next time. " +
						"Call this last, after attribution is complete.",
				),
				InputSchema: schemaFor(tools.GenerateFixPlanInput{}),
			},
			Handler: tools.GenerateFixPlan,
		},
	}
}

// toolParams extracts the Anthropic tool params from the tool definitions.
func toolParams(defs []toolDef) []anthropic.ToolUnionParam {
	params := make([]anthropic.ToolUnionParam, len(defs))
	for i, d := range defs {
		p := d.Param
		params[i] = anthropic.ToolUnionParam{OfTool: &p}
	}
	return params
}

// dispatch finds and executes the named tool, returning a JSON string result.
func dispatch(defs []toolDef, name string, inputJSON string) (string, error) {
	for _, d := range defs {
		if d.Param.Name == name {
			return d.Handler(inputJSON)
		}
	}
	return "", fmt.Errorf("unknown tool: %s", name)
}

// schemaFor generates a JSON schema compatible with the Anthropic tool input schema
// by introspecting the struct's JSON tags and jsonschema_description tags.
func schemaFor(v interface{}) anthropic.ToolInputSchemaParam {
	// We hand-code the schemas from known structs to avoid reflection complexity.
	// This keeps the tool definitions explicit and easy to audit.
	switch v.(type) {
	case tools.DetectRegressionInput:
		return buildSchema(
			map[string]interface{}{
				"description": prop("string", "Description of the bug, crash, or code change to analyze for regressions"),
				"files_changed": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "List of files modified in the change (optional)",
				},
				"environment":   prop("string", "Where the issue was found: ide, local_test, ci, code_review, staging, or production"),
				"error_message": prop("string", "The actual error or stack trace if available (optional)"),
			},
			[]string{"description", "environment"},
		)
	case tools.TriageIssueInput:
		return buildSchema(
			map[string]interface{}{
				"regression_type":        prop("string", "Type of regression from detect_regression output"),
				"severity":               prop("string", "Severity from detect_regression: critical, high, medium, or low"),
				"environment":            prop("string", "Where the issue was found: ide, local_test, ci, code_review, staging, or production"),
				"affected_users_estimate": map[string]interface{}{"type": "integer", "description": "Estimated number of users affected (0 if unknown)"},
			},
			[]string{"regression_type", "severity", "environment", "affected_users_estimate"},
		)
	case tools.AttributeIssueInput:
		return buildSchema(
			map[string]interface{}{
				"files_changed": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "List of files changed in the suspected commit or diff",
				},
				"description":     prop("string", "Description of the regression or bug"),
				"regression_type": prop("string", "Type of regression from detect_regression"),
			},
			[]string{"description", "regression_type"},
		)
	case tools.GenerateFixPlanInput:
		return buildSchema(
			map[string]interface{}{
				"regression_type": prop("string", "Type of regression from detect_regression"),
				"severity":        prop("string", "Severity level: critical, high, medium, or low"),
				"affected_files": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Files involved in the regression",
				},
				"root_cause": prop("string", "Description of the suspected root cause"),
				"priority":   prop("string", "Priority from triage: P0, P1, P2, or P3"),
			},
			[]string{"regression_type", "severity", "root_cause", "priority"},
		)
	}
	return anthropic.ToolInputSchemaParam{}
}

func prop(typ, description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        typ,
		"description": description,
	}
}

func buildSchema(properties map[string]interface{}, required []string) anthropic.ToolInputSchemaParam {
	raw, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	})
	var schema anthropic.ToolInputSchemaParam
	json.Unmarshal(raw, &schema)
	return schema
}
