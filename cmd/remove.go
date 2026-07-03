package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OSSMafia/gh-ghs/internal/ghcli"
	"github.com/OSSMafia/gh-ghs/internal/gitcfg"
	"github.com/OSSMafia/gh-ghs/internal/output"
)

var (
	flagRemoveLogout bool
	flagRemoveYes    bool
)

var removeCmd = &cobra.Command{
	Use:   "remove <profile>",
	Short: "Delete a profile and its pins (gh account stays logged in unless --logout)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		reg, hosts, err := loadWorld()
		if err != nil {
			return err
		}
		profile, err := requireProfile(reg, name)
		if err != nil {
			return err
		}

		pinCount := 0
		for _, p := range reg.Pins {
			if p.Profile == name {
				pinCount++
			}
		}
		q := fmt.Sprintf("Remove profile %q", name)
		if pinCount > 0 {
			q += fmt.Sprintf(" and its %d pin(s)", pinCount)
		}
		if !output.Confirm(q+"?", flagRemoveYes) {
			return output.Usagef("aborted (pass --yes to skip confirmation)")
		}

		delete(reg.Profiles, name)
		reg.RemovePinsForProfile(name)

		activeProfile, _, _ := reg.ProfileForUsername(hosts.Active)
		if err := gitcfg.Regenerate(reg, activeProfile); err != nil {
			return output.Envf("%v", err)
		}
		if err := reg.Save(); err != nil {
			return output.Envf("%v", err)
		}

		if flagRemoveLogout {
			if err := ghcli.EnsureUsable(); err != nil {
				return output.Envf("%v", err)
			}
			if err := ghcli.AuthLogout(profile.Username); err != nil {
				return output.Envf("gh auth logout failed: %v", err)
			}
		}

		if output.JSONMode() {
			return output.JSON(map[string]any{"removed": name, "logged_out": flagRemoveLogout})
		}
		fmt.Printf("Removed profile %s.\n", name)
		if !flagRemoveLogout {
			fmt.Println(output.Dim("gh account " + profile.Username + " is still logged in (use --logout to also remove it)."))
		}
		return nil
	},
}

func init() {
	removeCmd.Flags().BoolVar(&flagRemoveLogout, "logout", false, "also log the account out of gh")
	removeCmd.Flags().BoolVar(&flagRemoveYes, "yes", false, "skip confirmation")
	rootCmd.AddCommand(removeCmd)
}
