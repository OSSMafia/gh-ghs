package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattylight22/gh-ghs/internal/ghcli"
	"github.com/mattylight22/gh-ghs/internal/gitcfg"
	"github.com/mattylight22/gh-ghs/internal/output"
	"github.com/mattylight22/gh-ghs/internal/paths"
)

var flagUsePin bool

var useCmd = &cobra.Command{
	Use:   "use <profile>",
	Short: "Switch the active GitHub account and git identity to a profile",
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
		if err := ghcli.EnsureUsable(); err != nil {
			return output.Envf("%v", err)
		}

		loggedIn := false
		for _, u := range hosts.Users {
			if u == profile.Username {
				loggedIn = true
			}
		}
		if !loggedIn {
			return output.Envf("gh has no logged-in account %q — run `ghs add %s` first", profile.Username, name)
		}

		if hosts.Active != profile.Username {
			if err := ghcli.AuthSwitch(profile.Username); err != nil {
				return output.Envf("%v", err)
			}
		}

		if err := gitcfg.EnsureSetup(reg); err != nil {
			return output.Envf("gitconfig setup failed: %v", err)
		}
		if err := gitcfg.Regenerate(reg, name); err != nil {
			return output.Envf("regenerating ghs.gitconfig failed: %v", err)
		}
		if err := reg.Save(); err != nil {
			return output.Envf("saving config failed: %v", err)
		}

		if flagUsePin {
			dir, err := cwd()
			if err != nil {
				return err
			}
			canon, err := paths.Canonicalize(dir)
			if err != nil {
				return output.Envf("%v", err)
			}
			reg.SetPin(name, canon)
			if err := gitcfg.Regenerate(reg, name); err != nil {
				return output.Envf("%v", err)
			}
			if err := reg.Save(); err != nil {
				return output.Envf("%v", err)
			}
		}

		if output.JSONMode() {
			return output.JSON(map[string]any{"active_profile": name, "active_user": profile.Username, "pinned": flagUsePin})
		}
		fmt.Printf("Now using %s (%s <%s>)\n", output.Bold(name), profile.Username, profile.Email)
		if flagUsePin {
			fmt.Println("Pinned the current directory to " + name + ".")
		}
		fmt.Println(output.Dim("Note: switching accounts never touches branches or uncommitted work — only identity and push auth."))

		// Surface the footgun this machine has today: git not using gh for auth.
		if !credentialHelperUsesGh() {
			fmt.Println(output.Yellow("! git is not using gh as its credential helper — pushes may authenticate as a stale account."))
			fmt.Println(output.Yellow("  Run `ghs doctor --fix` to wire it up (runs `gh auth setup-git`)."))
		}
		return nil
	},
}

func credentialHelperUsesGh() bool {
	for _, h := range allCredentialHelpers() {
		if containsGhCredential(h) {
			return true
		}
	}
	return false
}

func init() {
	useCmd.Flags().BoolVar(&flagUsePin, "pin", false, "also pin the current directory to this profile")
	rootCmd.AddCommand(useCmd)
}
