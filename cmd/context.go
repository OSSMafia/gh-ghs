package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OSSMafia/gh-ghs/internal/status"
)

var flagContextMarkdown bool

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Emit a snippet for CLAUDE.md / agent rules describing this directory's account setup",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := contextText()
		if err != nil {
			return err
		}
		if flagContextMarkdown {
			fmt.Print("## GitHub account (managed by ghs)\n\n" + body)
		} else {
			fmt.Print(body)
		}
		return nil
	},
}

// contextText builds the agent-facing description of the current
// directory's account rules. Shared with `ghs init cursor`.
func contextText() (string, error) {
	reg, hosts, err := loadWorld()
	if err != nil {
		return "", err
	}
	dir, err := cwd()
	if err != nil {
		return "", err
	}
	r := status.Full(reg, hosts, dir)

	var b strings.Builder
	if r.Pin != nil {
		p := reg.Profiles[r.Pin.Profile]
		fmt.Fprintf(&b, "This directory tree (%s) is pinned to the GitHub profile %q:\n", r.Pin.Path, r.Pin.Profile)
		fmt.Fprintf(&b, "- Commits are authored as %s <%s>.\n", p.Name, p.Email)
		fmt.Fprintf(&b, "- Pushes authenticate as the GitHub account %q regardless of the globally active account.\n", p.Username)
	} else if r.ActiveProfile != "" {
		p := reg.Profiles[r.ActiveProfile]
		fmt.Fprintf(&b, "This directory is not pinned; it follows the globally active GitHub profile (currently %q: %s <%s>).\n",
			r.ActiveProfile, p.Username, p.Email)
	} else {
		b.WriteString("GitHub accounts on this machine are managed by ghs, but no profile is active here.\n")
	}
	b.WriteString("\nRules for agents:\n")
	b.WriteString("- Do NOT modify git identity settings (user.name, user.email, credential.*) directly; use `ghs` commands instead.\n")
	b.WriteString("- Before `git push`, run `ghs status --quiet`; a non-zero exit means the wrong account would be used — stop and report it.\n")
	b.WriteString("- `ghs status --json` describes the current account state machine-readably.\n")
	return b.String(), nil
}

func init() {
	contextCmd.Flags().BoolVar(&flagContextMarkdown, "markdown", false, "wrap in a markdown section heading")
	rootCmd.AddCommand(contextCmd)
}
