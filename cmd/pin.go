package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattylight22/gh-ghs/internal/gitcfg"
	"github.com/mattylight22/gh-ghs/internal/output"
	"github.com/mattylight22/gh-ghs/internal/paths"
)

var pinCmd = &cobra.Command{
	Use:   "pin <profile> [<dir>]",
	Short: "Pin a directory tree to a profile (identity AND push token, stateless)",
	Long: "Pinning writes a git includeIf block so every repo under the directory uses the\n" +
		"profile's identity and — via credential.username — the profile's push token,\n" +
		"regardless of which account is globally active.",
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		reg, hosts, err := loadWorld()
		if err != nil {
			return err
		}
		if _, err := requireProfile(reg, name); err != nil {
			return err
		}

		dir := ""
		if len(args) == 2 {
			dir = args[1]
		} else {
			dir, err = cwd()
			if err != nil {
				return err
			}
		}
		canon, err := paths.Canonicalize(dir)
		if err != nil {
			return output.Usagef("%v", err)
		}

		// Nested-pin awareness: informational, deeper pin wins deterministically.
		var enclosing []string
		for _, p := range reg.Pins {
			if p.Path != canon && (strings.HasPrefix(canon, p.Path+"/") || strings.HasPrefix(p.Path, canon+"/")) {
				enclosing = append(enclosing, fmt.Sprintf("%s -> %s", p.Path, p.Profile))
			}
		}

		reg.SetPin(name, canon)
		if err := gitcfg.EnsureSetup(reg); err != nil {
			return output.Envf("%v", err)
		}
		activeProfile, _, _ := reg.ProfileForUsername(hosts.Active)
		if err := gitcfg.Regenerate(reg, activeProfile); err != nil {
			return output.Envf("%v", err)
		}
		if err := reg.Save(); err != nil {
			return output.Envf("%v", err)
		}

		if output.JSONMode() {
			return output.JSON(map[string]any{"pinned": canon, "profile": name})
		}
		fmt.Printf("Pinned %s -> %s\n", canon, output.Bold(name))
		for _, e := range enclosing {
			fmt.Println(output.Dim("note: overlapping pin " + e + " (the deeper pin wins)"))
		}
		return nil
	},
}

var unpinCmd = &cobra.Command{
	Use:   "unpin [<dir>]",
	Short: "Remove the pin for a directory",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, hosts, err := loadWorld()
		if err != nil {
			return err
		}
		dir := ""
		if len(args) == 1 {
			dir = args[0]
		} else {
			dir, err = cwd()
			if err != nil {
				return err
			}
		}
		canon, err := paths.Canonicalize(dir)
		if err != nil {
			return output.Usagef("%v", err)
		}
		if !reg.RemovePin(canon) {
			if p := reg.PinFor(canon); p != nil {
				return output.Usagef("no pin at %s exactly — the covering pin is %s -> %s (unpin that path)", canon, p.Path, p.Profile)
			}
			return output.Usagef("no pin at %s", canon)
		}
		activeProfile, _, _ := reg.ProfileForUsername(hosts.Active)
		if err := gitcfg.Regenerate(reg, activeProfile); err != nil {
			return output.Envf("%v", err)
		}
		if err := reg.Save(); err != nil {
			return output.Envf("%v", err)
		}
		if output.JSONMode() {
			return output.JSON(map[string]any{"unpinned": canon})
		}
		fmt.Printf("Unpinned %s\n", canon)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pinCmd)
	rootCmd.AddCommand(unpinCmd)
}
