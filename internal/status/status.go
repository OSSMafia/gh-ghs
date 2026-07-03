// Package status computes the shared account/identity report that drives
// `ghs status`, `ghs prompt`, `ghs doctor`, and the guard hooks.
package status

import (
	"strings"

	"github.com/OSSMafia/gh-ghs/internal/ghcli"
	"github.com/OSSMafia/gh-ghs/internal/gitx"
	"github.com/OSSMafia/gh-ghs/internal/paths"
	"github.com/OSSMafia/gh-ghs/internal/registry"
)

// Level classifies the overall state.
type Level string

const (
	LevelOK       Level = "ok"
	LevelWarn     Level = "warning"
	LevelMismatch Level = "mismatch"
)

// Report is the full picture for one directory.
type Report struct {
	ActiveUser    string        `json:"active_user"`              // gh's active account ("" if none)
	ActiveProfile string        `json:"active_profile,omitempty"` // ghs profile matching it
	Pin           *registry.Pin `json:"pin,omitempty"`            // deepest pin covering the dir
	InRepo        bool          `json:"in_repo"`
	GitDir        string        `json:"git_dir,omitempty"`
	EffectiveName string        `json:"effective_name,omitempty"`
	EffectiveMail string        `json:"effective_email,omitempty"`
	EffProfile    string        `json:"effective_profile,omitempty"` // "" = unmanaged identity
	WorktreeTrap  bool          `json:"worktree_trap,omitempty"`
	Level         Level         `json:"level"`
	Notes         []string      `json:"notes,omitempty"`
}

// Fast computes the prompt-path report: no subprocesses, only file reads.
// It can't see repo-local overrides — that's Full's job.
func Fast(reg *registry.Config, hosts ghcli.Hosts, dir string) Report {
	r := Report{ActiveUser: hosts.Active, Level: LevelOK}
	if name, _, ok := reg.ProfileForUsername(hosts.Active); ok {
		r.ActiveProfile = name
	}
	// Best-effort canonicalization; prompt must never error.
	if canon, err := paths.Canonicalize(dir); err == nil {
		dir = canon
	}
	r.Pin = reg.PinFor(dir)
	if hosts.Active == "" {
		r.Level = LevelWarn
		r.Notes = append(r.Notes, "no gh account logged in")
		return r
	}
	if r.Pin != nil && r.Pin.Profile != r.ActiveProfile {
		// Pinned dirs are protected (identity + credential.username), so this
		// is informational, not a mismatch.
		r.Notes = append(r.Notes, "pinned to "+r.Pin.Profile+"; global active is "+displayName(r.ActiveProfile, hosts.Active))
	}
	return r
}

// Full extends Fast with real git resolution: effective identity, repo
// context, and the worktree trap.
func Full(reg *registry.Config, hosts ghcli.Hosts, dir string) Report {
	r := Fast(reg, hosts, dir)

	info := gitx.Repo(dir)
	r.InRepo = info.InRepo
	if !info.InRepo {
		return r
	}
	r.GitDir = info.GitDir
	r.EffectiveName = gitx.ConfigGet(dir, "user.name")
	r.EffectiveMail = gitx.ConfigGet(dir, "user.email")
	if name, _, ok := reg.ProfileForEmail(r.EffectiveMail); ok {
		r.EffProfile = name
	}

	if r.Pin != nil {
		// Worktree trap: cwd sits under a pin, but the repo's gitdir lives
		// elsewhere, so the includeIf never matched.
		if !strings.HasPrefix(info.GitDir, r.Pin.Path+"/") && info.GitDir != r.Pin.Path {
			r.WorktreeTrap = true
			r.Level = LevelMismatch
			r.Notes = append(r.Notes, "worktree/repo gitdir is outside the pinned tree — pin does not apply here; its main repo is at "+info.GitDir)
			return r
		}
		if r.EffProfile != r.Pin.Profile {
			r.Level = LevelMismatch
			r.Notes = append(r.Notes, "pinned to "+r.Pin.Profile+" but effective identity is "+identityDesc(r)+" — a repo-local or foreign config overrides the pin")
			return r
		}
		return r
	}

	// Unpinned repo: identity must match the active account.
	if r.EffProfile == "" {
		r.Level = LevelWarn
		r.Notes = append(r.Notes, "identity "+identityDesc(r)+" does not match any ghs profile (unmanaged)")
		return r
	}
	if r.ActiveProfile != "" && r.EffProfile != r.ActiveProfile {
		r.Level = LevelMismatch
		r.Notes = append(r.Notes, "identity is profile "+r.EffProfile+" but active gh account is "+displayName(r.ActiveProfile, hosts.Active)+" — push would use "+r.ActiveProfile+"'s token; pin this dir or run `ghs use "+r.EffProfile+"`")
	}
	return r
}

// OK reports whether the state is safe (used by --quiet / guard hooks).
func (r Report) OK() bool {
	return r.Level != LevelMismatch
}

func identityDesc(r Report) string {
	if r.EffectiveMail == "" {
		return "(unset)"
	}
	return r.EffectiveMail
}

func displayName(profile, user string) string {
	if profile != "" {
		return profile
	}
	return user
}
