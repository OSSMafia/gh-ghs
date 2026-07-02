// Package gitx wraps the git subprocess calls ghs needs: resolving the
// effective identity for a directory and editing global config safely.
// All writes go through `git config` itself, never by editing files, and
// they respect GIT_CONFIG_GLOBAL so tests (and cautious sessions) can be
// fully sandboxed.
package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		return strings.TrimSpace(out.String()), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// ConfigGet resolves a config key as seen from dir (full resolution:
// system + global + includeIf + repo-local). Returns "" when unset.
func ConfigGet(dir, key string) string {
	out, err := git(dir, "config", "--get", key)
	if err != nil {
		return ""
	}
	return out
}

// GlobalGet reads a key from global scope only. --includes is explicit:
// git defaults it OFF for scoped reads, and ghs's identity lives behind an
// include. Conditional gitdir includes still never match when dir is not
// inside a repository.
func GlobalGet(dir, key string) string {
	out, err := git(dir, "config", "--global", "--includes", "--get", key)
	if err != nil {
		return ""
	}
	return out
}

// ConfigGetAll returns all values of a multivar as resolved from dir
// (every scope: system, global, includes, repo-local).
func ConfigGetAll(dir, key string) []string {
	out, err := git(dir, "config", "--get-all", key)
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// GlobalGetAll returns all values of a multivar in global scope.
func GlobalGetAll(key string) []string {
	out, err := git("", "config", "--global", "--get-all", key)
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// GlobalAdd appends a value to a global multivar.
func GlobalAdd(key, value string) error {
	_, err := git("", "config", "--global", "--add", key, value)
	return err
}

// GlobalUnsetExact removes all entries of key whose value is exactly value.
// Uses --fixed-value so no other entries (e.g. the user's own includes) can
// be affected. Missing key/value is not an error.
func GlobalUnsetExact(key, value string) error {
	_, err := git("", "config", "--global", "--fixed-value", "--unset-all", key, value)
	if err != nil && isUnsetMissing(err) {
		return nil
	}
	return err
}

func isUnsetMissing(err error) bool {
	// git exits 5 when unsetting a key that doesn't exist.
	var ee *exec.ExitError
	if ok := errorsAs(err, &ee); ok {
		return ee.ExitCode() == 5
	}
	return false
}

func errorsAs(err error, target **exec.ExitError) bool {
	for err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// RepoInfo describes the repository context of a directory.
type RepoInfo struct {
	InRepo    bool
	GitDir    string // absolute, resolved gitdir (worktrees: .git/worktrees/<x>)
	CommonDir string // main repo's .git dir (hooks live here)
}

// Repo inspects dir's repository context. Never errors: outside a repo it
// returns InRepo=false.
func Repo(dir string) RepoInfo {
	out, err := git(dir, "rev-parse", "--is-inside-work-tree", "--absolute-git-dir", "--git-common-dir")
	if err != nil {
		return RepoInfo{}
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 2 || lines[0] != "true" {
		return RepoInfo{}
	}
	info := RepoInfo{InRepo: true, GitDir: lines[1]}
	if len(lines) >= 3 {
		// --git-common-dir can be relative (".git" at the repo root);
		// anchor it to dir so recorded paths survive cwd changes.
		common := lines[2]
		if !filepath.IsAbs(common) {
			if abs, err := filepath.Abs(filepath.Join(dir, common)); err == nil {
				common = abs
			}
		}
		info.CommonDir = common
	}
	return info
}

// NonRepoDir returns a directory guaranteed to be outside any git
// repository, for probing global config without includeIf interference.
func NonRepoDir() string {
	d := os.TempDir()
	probe := d
	for {
		if !Repo(probe).InRepo {
			return probe
		}
		parent := strings.TrimSuffix(probe, "/")
		idx := strings.LastIndex(parent, "/")
		if idx <= 0 {
			return "/"
		}
		probe = parent[:idx]
	}
}
