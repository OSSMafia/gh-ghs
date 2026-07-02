package gitcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattylight22/gh-ghs/internal/gitx"
	"github.com/mattylight22/gh-ghs/internal/paths"
	"github.com/mattylight22/gh-ghs/internal/registry"
)

// sandbox redirects every config path (ghs and git global) into temp dirs
// so no test can touch the real machine.
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull) // keep the Xcode osxkeychain config out
	return home
}

func reg() *registry.Config {
	return &registry.Config{
		Version: 1,
		Profiles: map[string]registry.Profile{
			"personal": {Username: "mattylight22", Name: "Matt Nick", Email: "personal@example.com"},
			"work":     {Username: "matt-thesis", Name: "Matt Nick", Email: "matt@use-thesis.com"},
		},
	}
}

func TestRegenerateGolden(t *testing.T) {
	sandbox(t)
	r := reg()
	r.SetPin("work", "/u/work")
	r.SetPin("personal", "/u/work/oss") // deeper pin, must render after /u/work

	if err := Regenerate(r, "personal"); err != nil {
		t.Fatal(err)
	}
	main, _ := paths.GhsGitconfig()
	data, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	fragWork, _ := paths.ProfileFragment("work")
	fragPersonal, _ := paths.ProfileFragment("personal")
	want := managedHeader +
		"[user]\n\tname = Matt Nick\n\temail = personal@example.com\n" +
		"\n[includeIf \"gitdir:/u/work/\"]\n\tpath = " + fragWork + "\n" +
		"\n[includeIf \"gitdir:/u/work/oss/\"]\n\tpath = " + fragPersonal + "\n"
	if content != want {
		t.Errorf("ghs.gitconfig mismatch:\n--- got ---\n%s\n--- want ---\n%s", content, want)
	}

	fragData, err := os.ReadFile(fragWork)
	if err != nil {
		t.Fatal(err)
	}
	frag := string(fragData)
	for _, needle := range []string{
		"email = matt@use-thesis.com",
		`[credential "https://github.com"]`,
		"username = matt-thesis",
	} {
		if !strings.Contains(frag, needle) {
			t.Errorf("fragment missing %q:\n%s", needle, frag)
		}
	}
}

func TestRegenerateRemovesStaleFragments(t *testing.T) {
	sandbox(t)
	r := reg()
	r.SetPin("work", "/u/work")
	if err := Regenerate(r, "personal"); err != nil {
		t.Fatal(err)
	}
	r.RemovePinsForProfile("work")
	if err := Regenerate(r, "personal"); err != nil {
		t.Fatal(err)
	}
	frag, _ := paths.ProfileFragment("work")
	if _, err := os.Stat(frag); !os.IsNotExist(err) {
		t.Error("stale fragment not removed")
	}
}

func TestEnsureSetupAndRemoveInclude(t *testing.T) {
	home := sandbox(t)
	// Simulate the user's pre-existing global config.
	gitconfig := filepath.Join(home, "gitconfig")
	orig := "[user]\n\tname = Original\n\temail = original@example.com\n"
	if err := os.WriteFile(gitconfig, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}

	r := reg()
	if err := EnsureSetup(r); err != nil {
		t.Fatal(err)
	}
	if err := Regenerate(r, "work"); err != nil {
		t.Fatal(err)
	}

	// Backup captured verbatim.
	backup, err := os.ReadFile(r.State.FirstRunBackup)
	if err != nil || string(backup) != orig {
		t.Errorf("backup = %q, err=%v", backup, err)
	}

	// ghs's include shadows the original identity (real git resolution).
	if got := gitx.GlobalGet(home, "user.email"); got != "matt@use-thesis.com" {
		t.Errorf("global user.email = %q, want work email via include", got)
	}

	// Idempotent.
	if err := EnsureSetup(r); err != nil {
		t.Fatal(err)
	}
	if n := len(gitx.GlobalGetAll("include.path")); n != 1 {
		t.Errorf("include.path count = %d, want 1", n)
	}

	// A user-owned include must survive removal of ours.
	userInclude := filepath.Join(home, "user-include.gitconfig")
	os.WriteFile(userInclude, []byte("# user's own\n"), 0o600)
	if err := gitx.GlobalAdd("include.path", userInclude); err != nil {
		t.Fatal(err)
	}

	if err := RemoveInclude(); err != nil {
		t.Fatal(err)
	}
	// Original identity is live again; user include untouched.
	if got := gitx.GlobalGet(home, "user.email"); got != "original@example.com" {
		t.Errorf("after removal user.email = %q, want original", got)
	}
	remaining := gitx.GlobalGetAll("include.path")
	if len(remaining) != 1 || remaining[0] != userInclude {
		t.Errorf("user include lost: %v", remaining)
	}
}

func TestIncludeIsEffectiveDetectsShadowing(t *testing.T) {
	home := sandbox(t)
	r := reg()
	if err := EnsureSetup(r); err != nil {
		t.Fatal(err)
	}
	if err := Regenerate(r, "work"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := IncludeIsEffective(r, "work"); !ok {
		t.Fatal("expected effective include")
	}

	// A [user] appended after our include shadows it.
	f, _ := os.OpenFile(filepath.Join(home, "gitconfig"), os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString("[user]\n\temail = sneaky@example.com\n")
	f.Close()
	if ok, got := IncludeIsEffective(r, "work"); ok {
		t.Errorf("shadowing not detected (got %q)", got)
	}

	// ReorderInclude repairs it.
	if err := ReorderInclude(); err != nil {
		t.Fatal(err)
	}
	if ok, got := IncludeIsEffective(r, "work"); !ok {
		t.Errorf("reorder did not fix shadowing (got %q)", got)
	}
}
