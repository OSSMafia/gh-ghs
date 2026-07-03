package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OSSMafia/gh-ghs/internal/output"
	"github.com/OSSMafia/gh-ghs/internal/paths"
)

var flagLinkDir string

// linkCmd makes the one binary answer to all three spellings: the extension
// binary stays where gh manages it; `ghs` and `git-ghs` symlinks in a PATH
// dir enable bare `ghs` and git's `git-<name>` subcommand convention.
var linkCmd = &cobra.Command{
	Use:   "link",
	Short: "Create `ghs` and `git-ghs` symlinks so bare ghs and `git ghs` work",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		self, err := paths.Self()
		if err != nil {
			return output.Envf("cannot locate own binary: %v", err)
		}
		binDir := flagLinkDir
		if binDir == "" {
			binDir, err = paths.DefaultBinDir()
			if err != nil {
				return output.Envf("%v", err)
			}
		}
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return output.Envf("%v", err)
		}

		reg, _, err := loadWorld()
		if err != nil {
			return err
		}

		var created []string
		for _, name := range []string{"ghs", "git-ghs"} {
			dst := filepath.Join(binDir, name)
			if existing, err := os.Readlink(dst); err == nil {
				if existing == self {
					created = append(created, dst)
					continue
				}
				if !strings.Contains(filepath.Base(existing), "gh-ghs") {
					return output.Envf("%s exists and points elsewhere (%s) — refusing to overwrite", dst, existing)
				}
				os.Remove(dst) // stale ghs link (e.g. old version path)
			} else if _, statErr := os.Lstat(dst); statErr == nil {
				return output.Envf("%s exists and is not a symlink — refusing to overwrite", dst)
			}
			if err := os.Symlink(self, dst); err != nil {
				return output.Envf("symlink %s: %v", dst, err)
			}
			created = append(created, dst)
		}

		// Record for uninstall.
		linkSet := map[string]bool{}
		for _, l := range reg.State.Symlinks {
			linkSet[l] = true
		}
		for _, l := range created {
			if !linkSet[l] {
				reg.State.Symlinks = append(reg.State.Symlinks, l)
			}
		}
		if err := reg.Save(); err != nil {
			return output.Envf("%v", err)
		}

		if output.JSONMode() {
			return output.JSON(map[string]any{"symlinks": created, "target": self})
		}
		for _, l := range created {
			fmt.Printf("linked %s -> %s\n", l, self)
		}
		onPath := false
		for _, p := range filepath.SplitList(os.Getenv("PATH")) {
			if p == binDir {
				onPath = true
			}
		}
		if !onPath {
			fmt.Println(output.Yellow("! " + binDir + " is not on your PATH. Add to ~/.zshrc:"))
			fmt.Println("    export PATH=\"" + binDir + ":$PATH\"")
		}
		return nil
	},
}

func init() {
	linkCmd.Flags().StringVar(&flagLinkDir, "dir", "", "directory for symlinks (default ~/.local/bin)")
	rootCmd.AddCommand(linkCmd)
}
