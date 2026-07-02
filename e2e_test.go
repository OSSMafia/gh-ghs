package main_test

// End-to-end tests: run the real ghs binary against real git, with every
// path redirected into a sandbox (HOME, XDG_CONFIG_HOME, GH_CONFIG_DIR,
// GIT_CONFIG_GLOBAL, GIT_CONFIG_SYSTEM) and gh replaced by a stub via
// GHS_GH_BIN. Nothing on the host machine is read or written.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var ghsBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "ghs-e2e-*")
	if err != nil {
		panic(err)
	}
	ghsBin = filepath.Join(tmp, "gh-ghs")
	out, err := exec.Command("go", "build", "-o", ghsBin, ".").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// sandbox holds a fully isolated environment.
type sandbox struct {
	home string
	env  []string
	t    *testing.T
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ghDir := filepath.Join(home, "gh")
	if err := os.MkdirAll(ghDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeHosts(t, ghDir, "alice")

	// The user's pre-existing global gitconfig.
	gitconfig := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[user]\n\tname = Original\n\temail = original@sand.box\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Stub gh: reports 2.86.0 and rewrites hosts.yml on `auth switch`.
	stub := filepath.Join(home, "gh-stub")
	stubScript := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "gh version 2.86.0 (stub)"; exit 0; fi
if [ "$1" = "auth" ] && [ "$2" = "switch" ]; then
  user=""
  while [ $# -gt 0 ]; do
    if [ "$1" = "--user" ]; then user="$2"; fi
    shift
  done
  cat > "$GH_CONFIG_DIR/hosts.yml" <<EOF
github.com:
    users:
        alice:
        bob:
    user: $user
EOF
  exit 0
fi
exit 0
`
	if err := os.WriteFile(stub, []byte(stubScript), 0o755); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"GH_CONFIG_DIR="+ghDir,
		"GIT_CONFIG_GLOBAL="+gitconfig,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GHS_GH_BIN="+stub,
		"NO_COLOR=1",
	)
	return &sandbox{home: home, env: env, t: t}
}

func writeHosts(t *testing.T, ghDir, active string) {
	t.Helper()
	content := "github.com:\n    users:\n        alice:\n        bob:\n    user: " + active + "\n"
	if err := os.WriteFile(filepath.Join(ghDir, "hosts.yml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ghs runs the binary; fails the test on unexpected exit code.
func (s *sandbox) ghs(wantCode int, dir string, args ...string) (string, string) {
	s.t.Helper()
	return s.run(wantCode, dir, ghsBin, args...)
}

func (s *sandbox) git(dir string, args ...string) string {
	s.t.Helper()
	out, _ := s.run(0, dir, "git", args...)
	return strings.TrimSpace(out)
}

func (s *sandbox) run(wantCode int, dir, bin string, args ...string) (string, string) {
	s.t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = s.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		s.t.Fatalf("%s %v: %v", bin, args, err)
	}
	if code != wantCode {
		s.t.Fatalf("%s %v: exit %d, want %d\nstdout: %s\nstderr: %s",
			filepath.Base(bin), args, code, wantCode, stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

func (s *sandbox) addProfiles() {
	s.t.Helper()
	s.ghs(0, s.home, "add", "personal", "--username", "alice", "--name", "Alice A", "--email", "alice@example.com")
	s.ghs(0, s.home, "add", "work", "--username", "bob", "--name", "Bob B", "--email", "bob@work.com")
}

func (s *sandbox) initRepo(rel string) string {
	s.t.Helper()
	dir := filepath.Join(s.home, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.t.Fatal(err)
	}
	s.git(s.home, "init", "-q", dir)
	return dir
}

func TestUseSwitchesAccountAndIdentity(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()

	s.ghs(0, s.home, "use", "work")
	if got := s.git(s.home, "config", "--global", "--includes", "user.email"); got != "bob@work.com" {
		t.Errorf("global email = %q, want bob@work.com", got)
	}
	out, _ := s.ghs(0, s.home, "whoami")
	if !strings.Contains(out, "bob") || !strings.Contains(out, "work") {
		t.Errorf("whoami = %q", out)
	}

	s.ghs(0, s.home, "use", "personal")
	if got := s.git(s.home, "config", "--global", "--includes", "user.email"); got != "alice@example.com" {
		t.Errorf("global email = %q, want alice@example.com", got)
	}
	// The user's own [user] block is untouched underneath.
	data, _ := os.ReadFile(filepath.Join(s.home, "gitconfig"))
	if !strings.Contains(string(data), "original@sand.box") {
		t.Error("original [user] block was modified")
	}
}

func TestPinForcesIdentityAndCredentialUsername(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()
	s.ghs(0, s.home, "use", "personal")

	workTree := filepath.Join(s.home, "work")
	if err := os.MkdirAll(workTree, 0o755); err != nil {
		t.Fatal(err)
	}
	s.ghs(0, s.home, "pin", "work", workTree)
	repo := s.initRepo("work/repo")

	// The core promise, validated by real git: identity AND credential
	// username resolve to the pinned profile even though personal is active.
	if got := s.git(repo, "config", "user.email"); got != "bob@work.com" {
		t.Errorf("pinned repo email = %q, want bob@work.com", got)
	}
	if got := s.git(repo, "config", "credential.https://github.com.username"); got != "bob" {
		t.Errorf("pinned credential.username = %q, want bob", got)
	}
	// Outside the pin, the active profile applies.
	other := s.initRepo("elsewhere/repo")
	if got := s.git(other, "config", "user.email"); got != "alice@example.com" {
		t.Errorf("unpinned repo email = %q, want alice@example.com", got)
	}
}

func TestNestedPinDeepestWins(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()
	s.ghs(0, s.home, "use", "work")

	workTree := filepath.Join(s.home, "work")
	ossTree := filepath.Join(s.home, "work", "oss")
	if err := os.MkdirAll(ossTree, 0o755); err != nil {
		t.Fatal(err)
	}
	s.ghs(0, s.home, "pin", "work", workTree)
	s.ghs(0, s.home, "pin", "personal", ossTree)

	inWork := s.initRepo("work/repo")
	inOss := s.initRepo("work/oss/repo")
	if got := s.git(inWork, "config", "user.email"); got != "bob@work.com" {
		t.Errorf("work repo = %q", got)
	}
	if got := s.git(inOss, "config", "user.email"); got != "alice@example.com" {
		t.Errorf("nested oss repo = %q, want deeper pin to win", got)
	}
}

func TestStatusDetectsMismatch(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()
	s.ghs(0, s.home, "use", "personal")

	repo := s.initRepo("proj")
	// Repo-local identity says work while personal is active: mismatch.
	s.git(repo, "config", "user.email", "bob@work.com")

	s.ghs(1, repo, "status", "--quiet")
	out, _ := s.ghs(1, repo, "status", "--json")
	var r map[string]any
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("status --json is not valid JSON: %v\n%s", err, out)
	}
	if r["level"] != "mismatch" {
		t.Errorf("level = %v, want mismatch", r["level"])
	}

	// Aligning identity resolves it.
	s.git(repo, "config", "user.email", "alice@example.com")
	s.ghs(0, repo, "status", "--quiet")
}

func TestPromptShowsMismatchOnPinnedDir(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()
	s.ghs(0, s.home, "use", "personal")

	workTree := filepath.Join(s.home, "work")
	os.MkdirAll(workTree, 0o755)
	s.ghs(0, s.home, "pin", "work", workTree)

	out, _ := s.ghs(0, workTree, "prompt", "--format", "zsh")
	if !strings.Contains(out, "≠work") || !strings.Contains(out, "%F{red}") {
		t.Errorf("prompt = %q, want red mismatch segment", out)
	}
	out, _ = s.ghs(0, s.home, "prompt", "--format", "zsh")
	if !strings.Contains(out, "%F{green}") || strings.Contains(out, "≠") {
		t.Errorf("prompt outside pin = %q, want green", out)
	}
}

func TestWorktreeTrapDetected(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()
	s.ghs(0, s.home, "use", "work")

	pinned := filepath.Join(s.home, "work")
	os.MkdirAll(pinned, 0o755)
	s.ghs(0, s.home, "pin", "work", pinned)

	// Main repo OUTSIDE the pin, worktree INSIDE it.
	main := s.initRepo("outside/main")
	s.git(main, "-c", "user.email=x@y.z", "-c", "user.name=x", "commit", "--allow-empty", "-m", "init")
	wt := filepath.Join(pinned, "wt")
	s.git(main, "worktree", "add", wt)

	_, stderr := s.ghs(1, wt, "status")
	_ = stderr
	out, _ := s.ghs(1, wt, "status", "--json")
	if !strings.Contains(out, "worktree_trap") {
		t.Errorf("worktree trap not flagged:\n%s", out)
	}
}

func TestClaudeHookBlocksMismatchedPush(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()
	s.ghs(0, s.home, "use", "personal")

	repo := s.initRepo("proj")
	s.git(repo, "config", "user.email", "bob@work.com") // mismatch

	payload := func(command, dir string) string {
		b, _ := json.Marshal(map[string]any{
			"tool_name":  "Bash",
			"tool_input": map[string]string{"command": command},
			"cwd":        dir,
		})
		return string(b)
	}

	// Push in a mismatched repo: blocked (exit 2), reason on stderr.
	cmd := exec.Command(ghsBin, "hook", "claude")
	cmd.Dir = s.home
	cmd.Env = s.env
	cmd.Stdin = strings.NewReader(payload("git push origin main", repo))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 2 {
		t.Fatalf("expected exit 2, got %v (stderr: %s)", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "BLOCKED") {
		t.Errorf("stderr = %q", stderr.String())
	}

	// Non-git commands and matched pushes pass.
	for _, tc := range []struct{ command, dir string }{
		{"ls -la", repo},
		{"git status", repo},
	} {
		cmd := exec.Command(ghsBin, "hook", "claude")
		cmd.Dir = s.home
		cmd.Env = s.env
		cmd.Stdin = strings.NewReader(payload(tc.command, tc.dir))
		if err := cmd.Run(); err != nil {
			t.Errorf("command %q should pass: %v", tc.command, err)
		}
	}
}

func TestGuardHookLifecycle(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()
	s.ghs(0, s.home, "use", "personal")
	repo := s.initRepo("proj")

	s.ghs(0, repo, "guard", "install")
	hook := filepath.Join(repo, ".git", "hooks", "pre-push")
	data, err := os.ReadFile(hook)
	if err != nil || !strings.Contains(string(data), "ghs-guard v1") {
		t.Fatalf("hook not installed: %v", err)
	}

	s.ghs(0, repo, "guard", "check")
	s.git(repo, "config", "user.email", "bob@work.com")
	s.ghs(1, repo, "guard", "check")

	s.ghs(0, repo, "guard", "uninstall")
	if _, err := os.Stat(hook); !os.IsNotExist(err) {
		t.Error("hook not removed")
	}

	// Foreign hooks are never touched.
	os.WriteFile(hook, []byte("#!/bin/sh\n# someone else's hook\n"), 0o755)
	_, stderr := s.ghs(3, repo, "guard", "install")
	if !strings.Contains(stderr, "refusing") {
		t.Errorf("expected refusal, got: %s", stderr)
	}
}

func TestLinkAndGitSubcommand(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()
	binDir := filepath.Join(s.home, "bin")
	s.ghs(0, s.home, "link", "--dir", binDir)

	for _, name := range []string{"ghs", "git-ghs"} {
		if _, err := os.Stat(filepath.Join(binDir, name)); err != nil {
			t.Fatalf("%s symlink missing: %v", name, err)
		}
	}

	// `git ghs whoami` resolves through git's subcommand convention.
	cmd := exec.Command("git", "ghs", "whoami")
	cmd.Dir = s.home
	cmd.Env = append(append([]string{}, s.env...), "PATH="+binDir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "alice") {
		t.Errorf("git ghs whoami: %v\n%s", err, out)
	}
}

func TestUninstallRestoresEverything(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()
	s.ghs(0, s.home, "use", "work")

	workTree := filepath.Join(s.home, "work")
	os.MkdirAll(workTree, 0o755)
	s.ghs(0, s.home, "pin", "work", workTree)
	binDir := filepath.Join(s.home, "bin")
	s.ghs(0, s.home, "link", "--dir", binDir)
	repo := s.initRepo("proj")
	s.ghs(0, repo, "guard", "install")

	// Claude hook in a project settings file that also has user content.
	claudeDir := filepath.Join(repo, ".claude")
	os.MkdirAll(claudeDir, 0o755)
	settings := filepath.Join(claudeDir, "settings.json")
	os.WriteFile(settings, []byte(`{"model": "opus"}`), 0o644)
	s.ghs(0, repo, "init", "claude", "--install")
	data, _ := os.ReadFile(settings)
	if !strings.Contains(string(data), "hook claude") || !strings.Contains(string(data), `"model"`) {
		t.Fatalf("claude hook merge broken: %s", data)
	}

	// A user-owned include must survive uninstall.
	userInc := filepath.Join(s.home, "mine.gitconfig")
	os.WriteFile(userInc, []byte("# mine\n"), 0o600)
	s.git(s.home, "config", "--global", "--add", "include.path", userInc)

	s.ghs(0, s.home, "uninstall", "--yes")

	// Original identity restored, ghs include gone, user include intact.
	if got := s.git(s.home, "config", "--global", "--includes", "user.email"); got != "original@sand.box" {
		t.Errorf("email after uninstall = %q, want original@sand.box", got)
	}
	gitconfig, _ := os.ReadFile(filepath.Join(s.home, "gitconfig"))
	if strings.Contains(string(gitconfig), "ghs.gitconfig") {
		t.Error("ghs include still present")
	}
	if !strings.Contains(string(gitconfig), "mine.gitconfig") {
		t.Error("user include was removed")
	}
	// Config dir, symlinks, guard hook, claude hook all gone.
	if _, err := os.Stat(filepath.Join(s.home, ".config", "ghs")); !os.IsNotExist(err) {
		t.Error("config dir still present")
	}
	if _, err := os.Stat(filepath.Join(binDir, "ghs")); !os.IsNotExist(err) {
		t.Error("symlink still present")
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Error("guard hook still present")
	}
	data, _ = os.ReadFile(settings)
	if strings.Contains(string(data), "hook claude") {
		t.Error("claude hook still present")
	}
	if !strings.Contains(string(data), `"model"`) {
		t.Error("user's claude settings were damaged")
	}
}

func TestJSONOutputsParse(t *testing.T) {
	s := newSandbox(t)
	s.addProfiles()
	s.ghs(0, s.home, "use", "personal")

	for _, args := range [][]string{
		{"whoami", "--json"},
		{"list", "--json"},
		{"status", "--json"},
	} {
		out, _ := s.ghs(0, s.home, args...)
		var v any
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Errorf("%v: invalid JSON: %v\n%s", args, err, out)
		}
	}
}
