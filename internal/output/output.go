// Package output centralizes terminal I/O policy: JSON mode, color
// (NO_COLOR / --no-color / TTY aware), interactivity detection, and the
// exit-code contract (0 ok, 1 mismatch/check failure, 2 usage, 3
// environment problem).
package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Exit codes — documented in README; stable for agents and scripts.
const (
	CodeOK       = 0
	CodeMismatch = 1
	CodeUsage    = 2
	CodeEnv      = 3
)

// ExitError carries a specific exit code up to main.
type ExitError struct {
	Code int
	Msg  string
}

func (e ExitError) Error() string { return e.Msg }

// Envf builds an environment-problem error (exit 3).
func Envf(format string, a ...any) error {
	return ExitError{Code: CodeEnv, Msg: fmt.Sprintf(format, a...)}
}

// Failf builds a check-failure error (exit 1).
func Failf(format string, a ...any) error {
	return ExitError{Code: CodeMismatch, Msg: fmt.Sprintf(format, a...)}
}

// Usagef builds a usage error (exit 2).
func Usagef(format string, a ...any) error {
	return ExitError{Code: CodeUsage, Msg: fmt.Sprintf(format, a...)}
}

var (
	jsonMode  bool
	noColor   bool
	stdoutTTY = term.IsTerminal(int(os.Stdout.Fd()))
	stdinTTY  = term.IsTerminal(int(os.Stdin.Fd()))
)

// Configure sets global output flags (called from the root command).
func Configure(jsonOut, disableColor bool) {
	jsonMode = jsonOut
	noColor = disableColor
}

// JSONMode reports whether --json is active.
func JSONMode() bool { return jsonMode }

// Interactive reports whether prompting the user is possible and sane.
func Interactive() bool { return stdinTTY && stdoutTTY }

// ColorEnabled applies the NO_COLOR / flag / TTY policy.
func ColorEnabled() bool {
	if noColor || os.Getenv("NO_COLOR") != "" {
		return false
	}
	return stdoutTTY
}

// ANSI wrappers — no-op when color is off.
func Green(s string) string  { return wrap(s, "32") }
func Red(s string) string    { return wrap(s, "31") }
func Yellow(s string) string { return wrap(s, "33") }
func Dim(s string) string    { return wrap(s, "2") }
func Bold(s string) string   { return wrap(s, "1") }

func wrap(s, code string) string {
	if !ColorEnabled() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// JSON pretty-prints v to stdout.
func JSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Confirm asks a yes/no question. Non-interactive sessions get the
// assumeYes value — commands must pass their --yes flag through.
func Confirm(question string, assumeYes bool) bool {
	if assumeYes {
		return true
	}
	if !Interactive() {
		return false
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// Prompt reads one line of input with a default. Returns def when
// non-interactive.
func Prompt(label, def string) string {
	if !Interactive() {
		return def
	}
	if def != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}
