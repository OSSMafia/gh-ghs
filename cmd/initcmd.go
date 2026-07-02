package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattylight22/gh-ghs/internal/output"
	"github.com/mattylight22/gh-ghs/internal/paths"
)

var (
	flagInitInstall bool
	flagInitGlobal  bool
)

const (
	zshBegin = "# >>> ghs initialize >>>"
	zshEnd   = "# <<< ghs initialize <<<"
)

var initCmd = &cobra.Command{
	Use:   "init <zsh|claude|cursor>",
	Short: "Integrations: zsh prompt segment, Claude Code push guard, Cursor rules",
}

// ---- zsh -------------------------------------------------------------

func zshSnippet() string {
	return zshBegin + "\n" +
		"autoload -Uz add-zsh-hook\n" +
		"_ghs_prompt_update() { _GHS_SEGMENT=\"$(command ghs prompt --format zsh 2>/dev/null)\" }\n" +
		"add-zsh-hook precmd _ghs_prompt_update\n" +
		"setopt PROMPT_SUBST\n" +
		"RPROMPT='${_GHS_SEGMENT}'\"${RPROMPT:-}\"\n" +
		zshEnd + "\n"
}

var initZshCmd = &cobra.Command{
	Use:   "zsh",
	Short: "Print (or --install into ~/.zshrc) the prompt segment snippet",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !flagInitInstall {
			fmt.Print(zshSnippet())
			fmt.Fprintln(os.Stderr, "\n# add the above to ~/.zshrc, or run: ghs init zsh --install")
			return nil
		}
		home, err := paths.Home()
		if err != nil {
			return output.Envf("%v", err)
		}
		zshrc := filepath.Join(home, ".zshrc")
		existing, err := os.ReadFile(zshrc)
		if err != nil && !os.IsNotExist(err) {
			return output.Envf("%v", err)
		}
		if strings.Contains(string(existing), zshBegin) {
			fmt.Println("snippet already installed in ~/.zshrc")
			return nil
		}
		content := string(existing)
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + zshSnippet()
		if err := paths.WriteFileAtomic(zshrc, []byte(content), 0o644); err != nil {
			return output.Envf("%v", err)
		}
		reg, _, err := loadWorld()
		if err != nil {
			return err
		}
		reg.State.ZshrcSnippet = true
		if err := reg.Save(); err != nil {
			return output.Envf("%v", err)
		}
		fmt.Println("installed prompt snippet in ~/.zshrc (open a new shell to see it)")
		return nil
	},
}

// ---- claude ----------------------------------------------------------

// claudeHookCommand is the marker by which ghs recognizes its own hook
// entries in settings.json (also greppable by users).
func claudeHookCommand() string {
	return selfOrGhs() + " hook claude"
}

var initClaudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Print (or --install) a Claude Code PreToolUse hook that blocks wrong-account pushes",
	Long: "The hook runs before every Bash tool call; when the call is a git push/commit\n" +
		"and the directory's identity mismatches the active account, it blocks the call\n" +
		"with a message the agent can act on. --install merges into .claude/settings.json\n" +
		"(project) or, with --global, ~/.claude/settings.json.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		entry := map[string]any{
			"matcher": "Bash",
			"hooks": []any{
				map[string]any{"type": "command", "command": claudeHookCommand()},
			},
		}
		if !flagInitInstall {
			snippet := map[string]any{"hooks": map[string]any{"PreToolUse": []any{entry}}}
			return output.JSON(snippet)
		}

		var settingsPath string
		if flagInitGlobal {
			home, err := paths.Home()
			if err != nil {
				return output.Envf("%v", err)
			}
			settingsPath = filepath.Join(home, ".claude", "settings.json")
		} else {
			dir, err := cwd()
			if err != nil {
				return err
			}
			settingsPath = filepath.Join(dir, ".claude", "settings.json")
		}

		root := map[string]any{}
		if data, err := os.ReadFile(settingsPath); err == nil {
			if err := json.Unmarshal(data, &root); err != nil {
				return output.Envf("%s is not valid JSON — not touching it. Add manually:\n"+
					"  {\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Bash\",\"hooks\":[{\"type\":\"command\",\"command\":%q}]}]}}",
					settingsPath, claudeHookCommand())
			}
		} else if !os.IsNotExist(err) {
			return output.Envf("%v", err)
		}

		hooks, _ := root["hooks"].(map[string]any)
		if hooks == nil {
			hooks = map[string]any{}
			root["hooks"] = hooks
		}
		pre, _ := hooks["PreToolUse"].([]any)
		for _, e := range pre {
			if b, _ := json.Marshal(e); strings.Contains(string(b), " hook claude") {
				fmt.Println("ghs hook already present in " + settingsPath)
				return nil
			}
		}
		hooks["PreToolUse"] = append(pre, entry)

		data, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return output.Envf("%v", err)
		}
		if err := paths.WriteFileAtomic(settingsPath, append(data, '\n'), 0o644); err != nil {
			return output.Envf("%v", err)
		}

		reg, _, err := loadWorld()
		if err != nil {
			return err
		}
		seen := false
		for _, f := range reg.State.ClaudeHookFiles {
			if f == settingsPath {
				seen = true
			}
		}
		if !seen {
			reg.State.ClaudeHookFiles = append(reg.State.ClaudeHookFiles, settingsPath)
			if err := reg.Save(); err != nil {
				return output.Envf("%v", err)
			}
		}
		fmt.Println("installed Claude Code push guard in " + settingsPath)
		return nil
	},
}

// ---- cursor ----------------------------------------------------------

var initCursorCmd = &cobra.Command{
	Use:   "cursor",
	Short: "Print (or --install) a Cursor rules file describing this project's account setup",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := contextText()
		if err != nil {
			return err
		}
		content := "---\ndescription: GitHub account/identity rules managed by ghs\nalwaysApply: true\n---\n\n" + body
		if !flagInitInstall {
			fmt.Print(content)
			return nil
		}
		dir, err := cwd()
		if err != nil {
			return err
		}
		rulePath := filepath.Join(dir, ".cursor", "rules", "ghs-account.mdc")
		if err := paths.WriteFileAtomic(rulePath, []byte(content), 0o644); err != nil {
			return output.Envf("%v", err)
		}
		reg, _, err := loadWorld()
		if err != nil {
			return err
		}
		seen := false
		for _, f := range reg.State.CursorRuleFiles {
			if f == rulePath {
				seen = true
			}
		}
		if !seen {
			reg.State.CursorRuleFiles = append(reg.State.CursorRuleFiles, rulePath)
			if err := reg.Save(); err != nil {
				return output.Envf("%v", err)
			}
		}
		fmt.Println("installed Cursor rule at " + rulePath)
		return nil
	},
}

func init() {
	initCmd.PersistentFlags().BoolVar(&flagInitInstall, "install", false, "write the integration instead of printing it")
	initClaudeCmd.Flags().BoolVar(&flagInitGlobal, "global", false, "install into ~/.claude/settings.json instead of the project")
	initCmd.AddCommand(initZshCmd, initClaudeCmd, initCursorCmd)
	rootCmd.AddCommand(initCmd)
}
