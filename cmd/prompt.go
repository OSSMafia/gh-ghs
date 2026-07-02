package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattylight22/gh-ghs/internal/status"
)

var flagPromptFormat string

// promptCmd is the shell-prompt fast path: file reads only, never errors,
// never spawns subprocesses. Zsh %F color codes are used in zsh format so
// the segment composes into PROMPT/RPROMPT without raw ANSI issues.
var promptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Emit a compact prompt segment (fast; for PROMPT/RPROMPT integration)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, hosts, err := loadWorld()
		if err != nil {
			return nil // prompt must never break the shell
		}
		dir, err := cwd()
		if err != nil {
			return nil
		}
		r := status.Fast(reg, hosts, dir)
		if r.ActiveUser == "" {
			return nil // nothing useful to show
		}

		mismatch := r.Pin != nil && r.Pin.Profile != r.ActiveProfile
		label := r.ActiveUser
		if mismatch {
			label = r.ActiveUser + "≠" + r.Pin.Profile
		}

		switch flagPromptFormat {
		case "zsh":
			if mismatch {
				fmt.Printf("%%F{red}⎇ %s%%f", label)
			} else {
				fmt.Printf("%%F{green}⎇ %s%%f", label)
			}
		default:
			fmt.Printf("⎇ %s", label)
		}
		return nil
	},
}

func init() {
	promptCmd.Flags().StringVar(&flagPromptFormat, "format", "plain", "output format: zsh|plain")
	rootCmd.AddCommand(promptCmd)
}
