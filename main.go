// LaaS Ladybug — Fix Fast Agent
//
// A regression detection and triage agent inspired by Facebook's Fix Fast system.
// https://engineering.fb.com/2021/02/17/developer-tools/fix-fast/
//
// Usage:
//
//	export ANTHROPIC_API_KEY=your_key
//	echo "NPE crash in auth service after deploying v2.3.1" | go run . [environment]
//	go run . "null pointer in db/user.go after migration" staging
//	go run .   # reads from stdin interactively
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/emyjamalian/laas-ladybug/agent"
)

func main() {
	// Determine input: from args, pipe, or interactive prompt.
	var input string
	var environment string

	args := os.Args[1:]

	// Check for --help
	for _, a := range args {
		if a == "--help" || a == "-h" {
			printUsage()
			os.Exit(0)
		}
	}

	switch len(args) {
	case 0:
		// No args — read from stdin (pipe or interactive).
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Piped input.
			scanner := bufio.NewScanner(os.Stdin)
			var lines []string
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			input = strings.Join(lines, "\n")
		} else {
			// Interactive prompt.
			printBanner()
			fmt.Print("Describe the bug or paste a diff (press Enter twice when done):\n> ")
			scanner := bufio.NewScanner(os.Stdin)
			var lines []string
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" && len(lines) > 0 {
					break
				}
				lines = append(lines, line)
			}
			input = strings.Join(lines, "\n")
			environment = promptEnvironment()
		}
	case 1:
		input = args[0]
	default:
		// Last arg can be an environment name.
		last := strings.ToLower(args[len(args)-1])
		validEnvs := map[string]bool{
			"ide": true, "local_test": true, "ci": true,
			"code_review": true, "staging": true, "production": true,
		}
		if validEnvs[last] {
			input = strings.Join(args[:len(args)-1], " ")
			environment = last
		} else {
			input = strings.Join(args, " ")
		}
	}

	if strings.TrimSpace(input) == "" {
		fmt.Fprintln(os.Stderr, "error: no input provided. Run with --help for usage.")
		os.Exit(1)
	}

	// Inject the environment into the input if we detected one.
	if environment != "" {
		input = fmt.Sprintf("[Detected in: %s]\n\n%s", environment, input)
	}

	printBanner()

	a := agent.New()
	_, err := a.Run(context.Background(), input, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		os.Exit(1)
	}
}

func promptEnvironment() string {
	envs := []string{"ide", "local_test", "ci", "code_review", "staging", "production"}
	fmt.Println("\nWhere was this issue detected?")
	for i, e := range envs {
		fmt.Printf("  %d) %s\n", i+1, e)
	}
	fmt.Print("> ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		choice := strings.TrimSpace(scanner.Text())
		for i, e := range envs {
			if choice == fmt.Sprintf("%d", i+1) || strings.EqualFold(choice, e) {
				return e
			}
		}
	}
	return "production" // conservative default
}

func printBanner() {
	fmt.Println(`
 _                    _           _  _
| |    __ _  __ _ ___| |    __ _ | || |__  _   _  __ _
| |   / _' |/ _' / __| |   / _' || || '_ \| | | |/ _' |
| |__| (_| | (_| \__ \ |__| (_| || || |_) | |_| | (_| |
|_____\__,_|\__,_|___/_____\__,_||_||_.__/ \__,_|\__, |
                                                   |___/
Fix Fast Agent — Inspired by Facebook's regression detection system`)
	fmt.Println()
}

func printUsage() {
	fmt.Println(`LaaS Ladybug — Fix Fast Agent

USAGE:
  go run . "bug description" [environment]
  echo "bug description" | go run .
  go run .   # interactive mode

ENVIRONMENTS:
  ide, local_test, ci, code_review, staging, production

EXAMPLES:
  go run . "NPE in auth/login.go after v2.3 deploy" production
  go run . "slow query after adding user_preferences column" staging
  go run . "security: SQL injection in search handler" production
  echo "panic: runtime error: index out of range" | go run .

ENVIRONMENT VARIABLE:
  ANTHROPIC_API_KEY   Your Anthropic API key (required)`)
}
