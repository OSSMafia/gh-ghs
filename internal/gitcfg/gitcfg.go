// Package gitcfg manages everything ghs writes into git configuration:
// the single include line in the user's global gitconfig, the ghs-owned
// ghs.gitconfig (identity + includeIf pin blocks), and per-profile
// fragments. The design invariant: ghs never edits the user's own config
// beyond adding/removing exactly one include.path entry via `git config`.
package gitcfg

import (
	"fmt"
	"os"
	"strings"

	"github.com/mattylight22/gh-ghs/internal/gitx"
	"github.com/mattylight22/gh-ghs/internal/paths"
	"github.com/mattylight22/gh-ghs/internal/registry"
)

const managedHeader = "# Managed by ghs. Do not edit — regenerated from config.toml.\n"

// IncludePresent reports whether the global gitconfig already includes
// ghs.gitconfig.
func IncludePresent() (bool, error) {
	target, err := paths.GhsGitconfig()
	if err != nil {
		return false, err
	}
	for _, v := range gitx.GlobalGetAll("include.path") {
		if v == target {
			return true, nil
		}
	}
	return false, nil
}

// EnsureSetup performs idempotent first-run setup: config dirs, a backup of
// the global gitconfig, and the include line. Mutates global git config —
// callers gate this behind explicit user actions.
func EnsureSetup(reg *registry.Config) error {
	backupDir, err := paths.BackupDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	profilesDir, err := paths.ProfilesDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		return err
	}

	if reg.State.FirstRunBackup == "" {
		src, err := paths.GlobalGitconfig()
		if err != nil {
			return err
		}
		dst := backupDir + "/gitconfig.orig"
		if data, err := os.ReadFile(src); err == nil {
			if err := paths.WriteFileAtomic(dst, data, 0o600); err != nil {
				return err
			}
		} else if os.IsNotExist(err) {
			if err := paths.WriteFileAtomic(dst, []byte{}, 0o600); err != nil {
				return err
			}
		} else {
			return err
		}
		reg.State.FirstRunBackup = dst
	}

	present, err := IncludePresent()
	if err != nil {
		return err
	}
	if !present {
		target, err := paths.GhsGitconfig()
		if err != nil {
			return err
		}
		// Ensure the included file exists before referencing it.
		if _, statErr := os.Stat(target); os.IsNotExist(statErr) {
			if err := paths.WriteFileAtomic(target, []byte(managedHeader), 0o644); err != nil {
				return err
			}
		}
		if err := gitx.GlobalAdd("include.path", target); err != nil {
			return err
		}
		reg.State.IncludeAdded = true
	}
	return nil
}

// RemoveInclude deletes ghs's include line from the global gitconfig,
// touching nothing else.
func RemoveInclude() error {
	target, err := paths.GhsGitconfig()
	if err != nil {
		return err
	}
	return gitx.GlobalUnsetExact("include.path", target)
}

// Regenerate rewrites ghs.gitconfig and all profile fragments from the
// registry. activeProfile ("" allowed) provides the global [user] identity;
// pins render as includeIf blocks, shallow-to-deep so deeper pins win.
func Regenerate(reg *registry.Config, activeProfile string) error {
	var b strings.Builder
	b.WriteString(managedHeader)

	if p, ok := reg.Profiles[activeProfile]; ok {
		b.WriteString("[user]\n")
		fmt.Fprintf(&b, "\tname = %s\n", p.Name)
		fmt.Fprintf(&b, "\temail = %s\n", p.Email)
	}

	for _, pin := range reg.PinsSortedByDepth() {
		frag, err := paths.ProfileFragment(pin.Profile)
		if err != nil {
			return err
		}
		// Trailing slash: git treats "gitdir:/x/" as "gitdir:/x/**".
		fmt.Fprintf(&b, "\n[includeIf \"gitdir:%s/\"]\n\tpath = %s\n", pin.Path, frag)
	}

	main, err := paths.GhsGitconfig()
	if err != nil {
		return err
	}
	if err := paths.WriteFileAtomic(main, []byte(b.String()), 0o644); err != nil {
		return err
	}

	// Fragments: one per pinned profile; stale ones removed.
	pinned := reg.PinnedProfiles()
	for name := range pinned {
		p, ok := reg.Profiles[name]
		if !ok {
			continue
		}
		frag, err := paths.ProfileFragment(name)
		if err != nil {
			return err
		}
		content := managedHeader +
			"[user]\n" +
			fmt.Sprintf("\tname = %s\n", p.Name) +
			fmt.Sprintf("\temail = %s\n", p.Email) +
			fmt.Sprintf("[credential \"https://github.com\"]\n\tusername = %s\n", p.Username)
		if err := paths.WriteFileAtomic(frag, []byte(content), 0o644); err != nil {
			return err
		}
	}
	profilesDir, err := paths.ProfilesDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(profilesDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".gitconfig")
		if name == e.Name() {
			continue
		}
		if !pinned[name] {
			frag, _ := paths.ProfileFragment(name)
			os.Remove(frag)
		}
	}
	return nil
}

// IncludeIsEffective verifies (from a non-repo directory, so conditional
// includes stay inert) that the active profile's email is what global config
// resolves to — i.e. no later [user] block shadows ghs's include.
func IncludeIsEffective(reg *registry.Config, activeProfile string) (bool, string) {
	p, ok := reg.Profiles[activeProfile]
	if !ok {
		return true, ""
	}
	got := gitx.GlobalGet(gitx.NonRepoDir(), "user.email")
	return strings.EqualFold(got, p.Email), got
}

// ReorderInclude moves ghs's include back to the end of the global config
// (unset + re-add), restoring last-value-wins precedence.
func ReorderInclude() error {
	target, err := paths.GhsGitconfig()
	if err != nil {
		return err
	}
	if err := gitx.GlobalUnsetExact("include.path", target); err != nil {
		return err
	}
	return gitx.GlobalAdd("include.path", target)
}
