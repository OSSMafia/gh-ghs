package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattylight22/gh-ghs/internal/gitcfg"
	"github.com/mattylight22/gh-ghs/internal/gitx"
	"github.com/mattylight22/gh-ghs/internal/output"
	"github.com/mattylight22/gh-ghs/internal/paths"
)

var (
	flagUninstallYes        bool
	flagUninstallKeepBackup bool
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove everything ghs changed and restore the previous setup",
	Long: "Reverts every change ghs made, in order: the gitconfig include (your original\n" +
		"identity takes effect immediately), credential-helper wiring (only if ghs added it),\n" +
		"guard hooks, agent hooks, the zshrc snippet, symlinks, and ghs's own config.\n" +
		"gh accounts and tokens are never touched.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, _, err := loadWorld()
		if err != nil {
			return err
		}

		fmt.Println("This will:")
		fmt.Println("  1. Remove ghs's include line from your global gitconfig (original identity returns immediately)")
		if reg.State.SetupGitRan {
			fmt.Println("  2. Remove the gh credential-helper entries ghs added (restoring your previous push auth)")
		}
		if len(reg.State.GuardRepos) > 0 {
			fmt.Printf("  3. Remove %d ghs pre-push guard hook(s)\n", len(reg.State.GuardRepos))
		}
		if len(reg.State.ClaudeHookFiles) > 0 {
			fmt.Printf("  4. Remove ghs hooks from %d Claude Code settings file(s)\n", len(reg.State.ClaudeHookFiles))
		}
		if len(reg.State.CursorRuleFiles) > 0 {
			fmt.Printf("  5. Remove %d Cursor rule file(s)\n", len(reg.State.CursorRuleFiles))
		}
		if reg.State.ZshrcSnippet {
			fmt.Println("  6. Remove the ghs prompt snippet from ~/.zshrc")
		}
		if len(reg.State.Symlinks) > 0 {
			fmt.Printf("  7. Remove %d symlink(s): %s\n", len(reg.State.Symlinks), strings.Join(reg.State.Symlinks, ", "))
		}
		fmt.Println("  8. Delete ~/.config/ghs (profiles, pins, state)")
		fmt.Println("gh accounts/tokens are NOT touched.")
		if !output.Confirm("Proceed?", flagUninstallYes) {
			return output.Usagef("aborted (pass --yes to skip confirmation)")
		}

		var problems []string

		// 1. Include line — the single change to the user's gitconfig.
		if err := gitcfg.RemoveInclude(); err != nil {
			problems = append(problems, "include removal: "+err.Error())
		}

		// 2. Credential helper (only what ghs added via doctor --fix / setup).
		if reg.State.SetupGitRan {
			for _, key := range []string{
				"credential.https://github.com.helper",
				"credential.https://gist.github.com.helper",
			} {
				for _, v := range gitx.GlobalGetAll(key) {
					if v == "" || containsGhCredential(v) {
						if err := gitx.GlobalUnsetExact(key, v); err != nil {
							problems = append(problems, key+": "+err.Error())
						}
					}
				}
			}
		}

		// 3. Guard hooks (marker-checked).
		for _, hookPath := range reg.State.GuardRepos {
			if _, err := removeGuardHook(hookPath); err != nil {
				problems = append(problems, "guard hook "+hookPath+": "+err.Error())
			}
		}

		// 4. Claude hooks (entries containing " hook claude").
		for _, settingsPath := range reg.State.ClaudeHookFiles {
			if err := removeClaudeHook(settingsPath); err != nil {
				problems = append(problems, "claude hook "+settingsPath+": "+err.Error())
			}
		}

		// 5. Cursor rule files (ghs-named, safe to delete).
		for _, rulePath := range reg.State.CursorRuleFiles {
			if err := os.Remove(rulePath); err != nil && !os.IsNotExist(err) {
				problems = append(problems, "cursor rule "+rulePath+": "+err.Error())
			}
		}

		// 6. zshrc snippet (marker-delimited block).
		if reg.State.ZshrcSnippet {
			if err := removeZshSnippet(); err != nil {
				problems = append(problems, "zshrc: "+err.Error())
			}
		}

		// 7. Symlinks.
		for _, l := range reg.State.Symlinks {
			if err := os.Remove(l); err != nil && !os.IsNotExist(err) {
				problems = append(problems, "symlink "+l+": "+err.Error())
			}
		}

		// 8. Config dir.
		cfgDir, err := paths.ConfigDir()
		if err == nil {
			if flagUninstallKeepBackup && reg.State.FirstRunBackup != "" {
				if data, err := os.ReadFile(reg.State.FirstRunBackup); err == nil {
					home, _ := paths.Home()
					kept := home + "/ghs-gitconfig-backup.orig"
					if err := paths.WriteFileAtomic(kept, data, 0o600); err == nil {
						fmt.Println("kept gitconfig backup at " + kept)
					}
				}
			}
			if err := os.RemoveAll(cfgDir); err != nil {
				problems = append(problems, "config dir: "+err.Error())
			}
		}

		if len(problems) > 0 {
			fmt.Println(output.Yellow("Completed with issues:"))
			for _, p := range problems {
				fmt.Println("  ! " + p)
			}
		} else {
			fmt.Println(output.Green("ghs is fully unwound — your git identity and push auth are back to their pre-ghs state."))
		}
		fmt.Println("Finish by removing the binary: " + output.Bold("gh extension remove gh-ghs"))
		fmt.Println(output.Dim("gh accounts were not touched; `gh auth logout --user <name>` removes them if you want."))
		if len(problems) > 0 {
			return output.ExitError{Code: output.CodeMismatch}
		}
		return nil
	},
}

// removeClaudeHook strips ghs's entries from a Claude settings file, leaving
// everything else exactly as parsed.
func removeClaudeHook(settingsPath string) error {
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("not valid JSON, remove the ghs entry manually")
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	pre, _ := hooks["PreToolUse"].([]any)
	kept := make([]any, 0, len(pre))
	for _, e := range pre {
		if b, _ := json.Marshal(e); strings.Contains(string(b), " hook claude") {
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == len(pre) {
		return nil
	}
	if len(kept) > 0 {
		hooks["PreToolUse"] = kept
	} else {
		delete(hooks, "PreToolUse")
		if len(hooks) == 0 {
			delete(root, "hooks")
		}
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return paths.WriteFileAtomic(settingsPath, append(out, '\n'), 0o644)
}

func removeZshSnippet() error {
	home, err := paths.Home()
	if err != nil {
		return err
	}
	zshrc := home + "/.zshrc"
	data, err := os.ReadFile(zshrc)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	content := string(data)
	start := strings.Index(content, zshBegin)
	end := strings.Index(content, zshEnd)
	if start == -1 || end == -1 || end < start {
		return nil
	}
	end += len(zshEnd)
	if end < len(content) && content[end] == '\n' {
		end++
	}
	cleaned := strings.TrimRight(content[:start], "\n") + "\n" + strings.TrimLeft(content[end:], "\n")
	return paths.WriteFileAtomic(zshrc, []byte(cleaned), 0o644)
}

func init() {
	uninstallCmd.Flags().BoolVar(&flagUninstallYes, "yes", false, "skip confirmation")
	uninstallCmd.Flags().BoolVar(&flagUninstallKeepBackup, "keep-backup", false, "keep the original gitconfig backup in ~")
	rootCmd.AddCommand(uninstallCmd)
}
