// Package cmd implements the ghs command surface. The binary answers to
// `gh ghs`, `git ghs` (via the git-ghs symlink), and bare `ghs` identically.
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OSSMafia/gh-ghs/internal/ghcli"
	"github.com/OSSMafia/gh-ghs/internal/output"
	"github.com/OSSMafia/gh-ghs/internal/registry"
)

var (
	flagJSON    bool
	flagNoColor bool
)

var rootCmd = &cobra.Command{
	Use:           "ghs",
	Short:         "Switch between GitHub accounts safely (like nvm, for your GitHub identity)",
	Long:          "ghs switches the active GitHub account (via gh), keeps your git commit identity in sync,\nand lets you pin directory trees to a profile so the right identity AND push token apply\nthere no matter what is globally active.",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		output.Configure(flagJSON, flagNoColor)
	},
}

// Execute runs the CLI and returns a process exit code.
func Execute() int {
	err := rootCmd.Execute()
	if err == nil {
		return output.CodeOK
	}
	var ee output.ExitError
	if errors.As(err, &ee) {
		if ee.Msg != "" {
			fmt.Fprintln(os.Stderr, "ghs: "+ee.Msg)
		}
		return ee.Code
	}
	fmt.Fprintln(os.Stderr, "ghs: "+err.Error())
	return output.CodeUsage
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "machine-readable JSON output")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "disable ANSI colors")
}

// loadWorld gathers the registry and gh hosts state used by most commands.
func loadWorld() (*registry.Config, ghcli.Hosts, error) {
	reg, err := registry.Load()
	if err != nil {
		return nil, ghcli.Hosts{}, output.Envf("cannot load ghs config: %v", err)
	}
	hosts, err := ghcli.ReadHosts()
	if err != nil {
		return nil, ghcli.Hosts{}, output.Envf("cannot read gh hosts.yml: %v", err)
	}
	return reg, hosts, nil
}

func requireProfile(reg *registry.Config, name string) (registry.Profile, error) {
	p, ok := reg.Profiles[name]
	if !ok {
		known := reg.ProfileNames()
		if len(known) == 0 {
			return registry.Profile{}, output.Usagef("no profiles configured yet — run `ghs add`")
		}
		return registry.Profile{}, output.Usagef("unknown profile %q (known: %v)", name, known)
	}
	return p, nil
}

func cwd() (string, error) {
	d, err := os.Getwd()
	if err != nil {
		return "", output.Envf("cannot determine current directory: %v", err)
	}
	return d, nil
}
