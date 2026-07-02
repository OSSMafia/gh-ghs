package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/mattylight22/gh-ghs/internal/output"
	"github.com/mattylight22/gh-ghs/internal/status"
)

// pushOrCommitRe matches Bash commands that would commit or push. Broad on
// purpose: a false positive only costs a fast status check.
var pushOrCommitRe = regexp.MustCompile(`\bgit\b[^|;&]*\b(push|commit)\b`)

// hookCmd implements agent-hook adapters. Hidden: it exists to be called by
// hook runners (Claude Code PreToolUse), not by people.
var hookCmd = &cobra.Command{
	Use:    "hook <claude>",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if args[0] != "claude" {
			return output.Usagef("unknown hook adapter %q", args[0])
		}
		return runClaudeHook()
	},
}

// claudeHookInput is the subset of Claude Code's PreToolUse stdin payload
// that the guard needs.
type claudeHookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
	CWD string `json:"cwd"`
}

// runClaudeHook reads the PreToolUse payload from stdin. Exit 0 lets the
// tool call proceed; exit 2 blocks it and feeds stderr back to the agent.
// Anything unparseable is allowed through — the guard must never break
// unrelated tool calls.
func runClaudeHook() error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil
	}
	var in claudeHookInput
	if err := json.Unmarshal(data, &in); err != nil {
		return nil
	}
	if in.ToolName != "" && in.ToolName != "Bash" {
		return nil
	}
	if !pushOrCommitRe.MatchString(in.ToolInput.Command) {
		return nil
	}

	dir := in.CWD
	if dir == "" {
		dir, err = os.Getwd()
		if err != nil {
			return nil
		}
	}
	reg, hosts, err := loadWorld()
	if err != nil {
		return nil
	}
	r := status.Full(reg, hosts, dir)
	if r.OK() {
		return nil
	}
	fmt.Fprintln(os.Stderr, "BLOCKED by ghs: GitHub identity/account mismatch in "+dir)
	for _, n := range r.Notes {
		fmt.Fprintln(os.Stderr, "- "+n)
	}
	fmt.Fprintln(os.Stderr, "Resolve before pushing/committing: run `ghs status` for details; typically `ghs use <profile>` or `ghs pin <profile>` fixes this. Do not bypass by editing git config directly.")
	return output.ExitError{Code: output.CodeUsage} // exit 2 = blocking error for Claude Code hooks
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
