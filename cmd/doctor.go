package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattylight22/gh-ghs/internal/ghcli"
	"github.com/mattylight22/gh-ghs/internal/gitcfg"
	"github.com/mattylight22/gh-ghs/internal/gitx"
	"github.com/mattylight22/gh-ghs/internal/output"
	"github.com/mattylight22/gh-ghs/internal/paths"
)

var flagDoctorFix bool

type check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Fixed  bool   `json:"fixed,omitempty"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the account/identity setup; --fix repairs what it safely can",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, hosts, err := loadWorld()
		if err != nil {
			return err
		}
		var checks []check
		add := func(c check) { checks = append(checks, c) }

		// gh present + version.
		if !ghcli.Installed() {
			add(check{Name: "gh installed", OK: false, Detail: "GitHub CLI not found — brew install gh"})
		} else if v, err := ghcli.Version(); err != nil {
			add(check{Name: "gh installed", OK: false, Detail: err.Error()})
		} else {
			ok := ghcli.EnsureUsable() == nil
			add(check{Name: "gh installed", OK: ok, Detail: fmt.Sprintf("gh %d.%d.%d (need >= 2.40 for multi-account)", v[0], v[1], v[2])})
		}

		// Accounts.
		add(check{Name: "gh accounts", OK: len(hosts.Users) > 0,
			Detail: fmt.Sprintf("%d logged in, active: %s", len(hosts.Users), orNone(hosts.Active))})

		// Profiles.
		add(check{Name: "ghs profiles", OK: len(reg.Profiles) > 0,
			Detail: fmt.Sprintf("%d configured", len(reg.Profiles))})
		activeProfile, _, activeKnown := reg.ProfileForUsername(hosts.Active)
		if hosts.Active != "" && !activeKnown {
			add(check{Name: "active account has profile", OK: false,
				Detail: hosts.Active + " has no ghs profile — `ghs add`"})
		}

		// Include present + effective (only meaningful once profiles exist).
		if len(reg.Profiles) > 0 {
			present, _ := gitcfg.IncludePresent()
			c := check{Name: "gitconfig include", OK: present}
			if !present {
				c.Detail = "ghs.gitconfig is not included from the global gitconfig"
				if flagDoctorFix && activeKnown {
					if err := gitcfg.EnsureSetup(reg); err == nil {
						if err := gitcfg.Regenerate(reg, activeProfile); err == nil {
							_ = reg.Save()
							c.OK, c.Fixed = true, true
						}
					}
				}
			}
			add(c)

			if present && activeKnown {
				effective, got := gitcfg.IncludeIsEffective(reg, activeProfile)
				c := check{Name: "include ordering", OK: effective}
				if !effective {
					c.Detail = fmt.Sprintf("global user.email resolves to %q, expected %q — a later [user] block shadows ghs", got, reg.Profiles[activeProfile].Email)
					if flagDoctorFix {
						if err := gitcfg.ReorderInclude(); err == nil {
							if ok, _ := gitcfg.IncludeIsEffective(reg, activeProfile); ok {
								c.OK, c.Fixed = true, true
							}
						}
					}
				}
				add(c)
			}
		}

		// Credential helper wiring — the osxkeychain footgun.
		{
			usesGh := credentialHelperUsesGh()
			c := check{Name: "git push auth via gh", OK: usesGh}
			if !usesGh {
				c.Detail = "git's credential helper does not include gh — `gh auth switch` will NOT change what `git push` authenticates as"
				if flagDoctorFix {
					if output.Confirm("Run `gh auth setup-git` to make gh the credential helper for github.com?", false) {
						if err := ghcli.SetupGit(); err == nil {
							reg.State.SetupGitRan = true
							_ = reg.Save()
							c.OK, c.Fixed = true, true
						} else {
							c.Detail += "; setup-git failed: " + err.Error()
						}
					} else {
						c.Detail += " (fix declined/non-interactive; run `gh auth setup-git` yourself)"
					}
				}
			}
			add(c)
		}

		// Stale pins.
		var stale []string
		for _, p := range reg.Pins {
			if _, err := os.Stat(p.Path); err != nil {
				stale = append(stale, p.Path)
			}
		}
		c := check{Name: "pins", OK: len(stale) == 0,
			Detail: fmt.Sprintf("%d pins", len(reg.Pins))}
		if len(stale) > 0 {
			c.Detail = "missing directories: " + strings.Join(stale, ", ")
			if flagDoctorFix {
				for _, s := range stale {
					reg.RemovePin(s)
				}
				if err := gitcfg.Regenerate(reg, activeProfile); err == nil {
					_ = reg.Save()
					c.OK, c.Fixed = true, true
				}
			}
		}
		add(c)

		// Fragments exist for pinned profiles.
		missingFrag := []string{}
		for name := range reg.PinnedProfiles() {
			frag, _ := paths.ProfileFragment(name)
			if _, err := os.Stat(frag); err != nil {
				missingFrag = append(missingFrag, name)
			}
		}
		fc := check{Name: "profile fragments", OK: len(missingFrag) == 0}
		if len(missingFrag) > 0 {
			fc.Detail = "missing fragments for: " + strings.Join(missingFrag, ", ")
			if flagDoctorFix {
				if err := gitcfg.Regenerate(reg, activeProfile); err == nil {
					fc.OK, fc.Fixed = true, true
				}
			}
		}
		add(fc)

		// Symlinks recorded in state still resolve.
		broken := []string{}
		for _, l := range reg.State.Symlinks {
			if _, err := os.Stat(l); err != nil {
				broken = append(broken, l)
			}
		}
		if len(reg.State.Symlinks) > 0 {
			add(check{Name: "symlinks", OK: len(broken) == 0,
				Detail: ifThen(len(broken) > 0, "broken: "+strings.Join(broken, ", "), fmt.Sprintf("%d present", len(reg.State.Symlinks)))})
		}

		// Report.
		allOK := true
		for _, c := range checks {
			if !c.OK {
				allOK = false
			}
		}
		if output.JSONMode() {
			if err := output.JSON(map[string]any{"ok": allOK, "checks": checks}); err != nil {
				return err
			}
		} else {
			for _, c := range checks {
				mark := output.Green("✓")
				if !c.OK {
					mark = output.Red("✗")
				} else if c.Fixed {
					mark = output.Yellow("✓ (fixed)")
				}
				line := fmt.Sprintf("%s %s", mark, c.Name)
				if c.Detail != "" {
					line += output.Dim(" — " + c.Detail)
				}
				fmt.Println(line)
			}
		}
		if !allOK {
			return output.ExitError{Code: output.CodeMismatch}
		}
		return nil
	},
}

// allCredentialHelpers returns credential helpers effective for github.com
// from every config scope.
func allCredentialHelpers() []string {
	dir := gitx.NonRepoDir()
	helpers := gitx.ConfigGetAll(dir, "credential.helper")
	helpers = append(helpers, gitx.ConfigGetAll(dir, "credential.https://github.com.helper")...)
	return helpers
}

func containsGhCredential(helper string) bool {
	return strings.Contains(helper, "gh auth git-credential") ||
		strings.Contains(helper, "gh.exe auth git-credential")
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func ifThen(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func init() {
	doctorCmd.Flags().BoolVar(&flagDoctorFix, "fix", false, "repair fixable problems (asks before anything that changes auth)")
	rootCmd.AddCommand(doctorCmd)
}
