package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattylight22/gh-ghs/internal/output"
	"github.com/mattylight22/gh-ghs/internal/status"
)

var flagQuiet bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show active account, effective identity, pin state, and any mismatch",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, hosts, err := loadWorld()
		if err != nil {
			return err
		}
		dir, err := cwd()
		if err != nil {
			return err
		}
		r := status.Full(reg, hosts, dir)

		if flagQuiet {
			if !r.OK() {
				return output.ExitError{Code: output.CodeMismatch}
			}
			return nil
		}
		if output.JSONMode() {
			if err := output.JSON(r); err != nil {
				return err
			}
		} else {
			printHumanStatus(r)
		}
		if !r.OK() {
			return output.ExitError{Code: output.CodeMismatch}
		}
		return nil
	},
}

func printHumanStatus(r status.Report) {
	account := r.ActiveUser
	if account == "" {
		account = output.Red("(none — run `ghs add`)")
	} else if r.ActiveProfile != "" {
		account = fmt.Sprintf("%s (profile: %s)", output.Bold(r.ActiveUser), r.ActiveProfile)
	}
	fmt.Printf("Account : %s  %s\n", account, output.Dim("[gh active]"))

	if r.InRepo {
		identity := r.EffectiveMail
		if identity == "" {
			identity = "(unset)"
		}
		if r.EffProfile != "" {
			identity = fmt.Sprintf("%s (profile: %s)", identity, r.EffProfile)
		}
		fmt.Printf("Identity: %s\n", identity)
	} else {
		fmt.Printf("Identity: %s\n", output.Dim("(not in a git repository)"))
	}

	if r.Pin != nil {
		fmt.Printf("Pin     : %s -> %s\n", r.Pin.Path, output.Bold(r.Pin.Profile))
	} else {
		fmt.Printf("Pin     : %s\n", output.Dim("(none)"))
	}

	switch r.Level {
	case status.LevelOK:
		if r.Pin != nil {
			fmt.Printf("Push    : %s\n", output.Green("OK — pinned dir forces "+r.Pin.Profile+"'s token via credential.username"))
		} else {
			fmt.Printf("Push    : %s\n", output.Green("OK"))
		}
	case status.LevelWarn:
		fmt.Printf("Push    : %s\n", output.Yellow("WARNING"))
	case status.LevelMismatch:
		fmt.Printf("Push    : %s\n", output.Red("MISMATCH"))
	}
	for _, n := range r.Notes {
		fmt.Printf("  %s %s\n", output.Yellow("!"), n)
	}
}

func init() {
	statusCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false, "no output; exit 0 if safe, 1 on mismatch")
	rootCmd.AddCommand(statusCmd)
}
