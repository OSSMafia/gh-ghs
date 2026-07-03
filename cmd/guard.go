package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OSSMafia/gh-ghs/internal/gitx"
	"github.com/OSSMafia/gh-ghs/internal/output"
	"github.com/OSSMafia/gh-ghs/internal/paths"
	"github.com/OSSMafia/gh-ghs/internal/status"
)

const guardMarker = "# ghs-guard v1"

var guardCmd = &cobra.Command{
	Use:   "guard <install|uninstall|check>",
	Short: "Opt-in per-repo pre-push hook that blocks pushes on identity mismatch",
	Long: "guard install writes a pre-push hook into the CURRENT repository only.\n" +
		"It never touches other repos, never installs globally, and refuses to\n" +
		"overwrite a hook it didn't create.",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"install", "uninstall", "check"},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "check":
			return guardCheck()
		case "install":
			return guardInstall()
		case "uninstall":
			return guardUninstall()
		default:
			return output.Usagef("unknown guard action %q", args[0])
		}
	},
}

func guardCheck() error {
	reg, hosts, err := loadWorld()
	if err != nil {
		return err
	}
	dir, err := cwd()
	if err != nil {
		return err
	}
	r := status.Full(reg, hosts, dir)
	if !r.OK() {
		for _, n := range r.Notes {
			fmt.Fprintln(os.Stderr, "ghs guard: "+n)
		}
		return output.ExitError{Code: output.CodeMismatch}
	}
	return nil
}

func hooksDir(dir string) (string, error) {
	info := gitx.Repo(dir)
	if !info.InRepo {
		return "", output.Usagef("not inside a git repository")
	}
	// Hooks live in the common dir so a guard covers all worktrees.
	common := info.CommonDir
	if common == "" {
		common = info.GitDir
	}
	return filepath.Join(common, "hooks"), nil
}

func guardInstall() error {
	dir, err := cwd()
	if err != nil {
		return err
	}
	hooks, err := hooksDir(dir)
	if err != nil {
		return err
	}
	hookPath := filepath.Join(hooks, "pre-push")
	if data, err := os.ReadFile(hookPath); err == nil {
		if strings.Contains(string(data), guardMarker) {
			fmt.Println("guard already installed at " + hookPath)
			return nil
		}
		return output.Envf("a pre-push hook already exists at %s and it isn't ghs's — refusing to overwrite.\n"+
			"Add this line to it manually instead:\n  %s guard check || exit 1", hookPath, selfOrGhs())
	}

	script := "#!/bin/sh\n" +
		guardMarker + " — installed by ghs; `ghs guard uninstall` removes it.\n" +
		"\"" + selfOrGhs() + "\" guard check || {\n" +
		"  echo \"push blocked: fix with 'ghs status' (or bypass once with --no-verify)\" >&2\n" +
		"  exit 1\n" +
		"}\n"
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		return output.Envf("%v", err)
	}
	if err := paths.WriteFileAtomic(hookPath, []byte(script), 0o755); err != nil {
		return output.Envf("%v", err)
	}

	reg, _, err := loadWorld()
	if err != nil {
		return err
	}
	seen := false
	for _, r := range reg.State.GuardRepos {
		if r == hookPath {
			seen = true
		}
	}
	if !seen {
		reg.State.GuardRepos = append(reg.State.GuardRepos, hookPath)
		if err := reg.Save(); err != nil {
			return output.Envf("%v", err)
		}
	}
	if output.JSONMode() {
		return output.JSON(map[string]string{"installed": hookPath})
	}
	fmt.Println("guard installed: " + hookPath)
	return nil
}

func guardUninstall() error {
	dir, err := cwd()
	if err != nil {
		return err
	}
	hooks, err := hooksDir(dir)
	if err != nil {
		return err
	}
	hookPath := filepath.Join(hooks, "pre-push")
	removed, err := removeGuardHook(hookPath)
	if err != nil {
		return err
	}
	if !removed {
		return output.Usagef("no ghs guard hook at %s", hookPath)
	}
	reg, _, err := loadWorld()
	if err != nil {
		return err
	}
	kept := reg.State.GuardRepos[:0]
	for _, r := range reg.State.GuardRepos {
		if r != hookPath {
			kept = append(kept, r)
		}
	}
	reg.State.GuardRepos = kept
	if err := reg.Save(); err != nil {
		return output.Envf("%v", err)
	}
	fmt.Println("guard removed: " + hookPath)
	return nil
}

// removeGuardHook deletes hookPath only if it contains the ghs marker.
func removeGuardHook(hookPath string) (bool, error) {
	data, err := os.ReadFile(hookPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, output.Envf("%v", err)
	}
	if !strings.Contains(string(data), guardMarker) {
		return false, output.Envf("%s exists but was not installed by ghs — leaving it alone", hookPath)
	}
	if err := os.Remove(hookPath); err != nil {
		return false, output.Envf("%v", err)
	}
	return true, nil
}

// selfOrGhs returns the absolute binary path for use inside hook scripts,
// where PATH may not include ~/.local/bin.
func selfOrGhs() string {
	if self, err := paths.Self(); err == nil {
		return self
	}
	return "ghs"
}

func init() {
	rootCmd.AddCommand(guardCmd)
}
