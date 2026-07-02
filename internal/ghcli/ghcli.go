// Package ghcli reads GitHub CLI state (hosts.yml) and wraps gh subprocess
// invocations. Reads never spawn a process — critical for prompt latency.
package ghcli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	ghconfig "github.com/cli/go-gh/v2/pkg/config"
)

// DefaultHost is the host ghs manages. Enterprise hosts are out of scope for v0.1.
const DefaultHost = "github.com"

// MinVersion is the oldest gh release with multi-account support.
var MinVersion = [3]int{2, 40, 0}

// Hosts is the subset of gh's hosts.yml that ghs needs.
type Hosts struct {
	Active string   // active account login for DefaultHost ("" if none)
	Users  []string // all logged-in account logins for DefaultHost
}

// HostsPath returns the hosts.yml location using gh's own precedence
// (GH_CONFIG_DIR, XDG_CONFIG_HOME, HOME).
func HostsPath() string {
	return filepath.Join(ghconfig.ConfigDir(), "hosts.yml")
}

// ReadHosts parses hosts.yml. A missing file yields an empty Hosts, not an
// error — that simply means no accounts are logged in.
func ReadHosts() (Hosts, error) {
	data, err := os.ReadFile(HostsPath())
	if os.IsNotExist(err) {
		return Hosts{}, nil
	}
	if err != nil {
		return Hosts{}, err
	}
	cfg := ghconfig.ReadFromString(string(data))
	var h Hosts
	if active, err := cfg.Get([]string{DefaultHost, "user"}); err == nil {
		h.Active = active
	}
	if users, err := cfg.Keys([]string{DefaultHost, "users"}); err == nil {
		h.Users = users
	}
	// Older single-account layouts may lack a users map.
	if len(h.Users) == 0 && h.Active != "" {
		h.Users = []string{h.Active}
	}
	return h, nil
}

// Bin returns the gh executable to invoke. GHS_GH_BIN overrides for tests.
func Bin() string {
	if b := os.Getenv("GHS_GH_BIN"); b != "" {
		return b
	}
	return "gh"
}

// Installed reports whether gh is on PATH (or overridden).
func Installed() bool {
	_, err := exec.LookPath(Bin())
	return err == nil
}

var versionRe = regexp.MustCompile(`gh version (\d+)\.(\d+)\.(\d+)`)

// Version returns gh's semantic version.
func Version() ([3]int, error) {
	out, err := exec.Command(Bin(), "--version").Output()
	if err != nil {
		return [3]int{}, fmt.Errorf("gh not runnable: %w", err)
	}
	m := versionRe.FindStringSubmatch(string(out))
	if m == nil {
		return [3]int{}, fmt.Errorf("cannot parse gh version from %q", strings.SplitN(string(out), "\n", 2)[0])
	}
	var v [3]int
	for i := 0; i < 3; i++ {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, nil
}

// EnsureUsable verifies gh is installed and >= MinVersion.
func EnsureUsable() error {
	if !Installed() {
		return fmt.Errorf("GitHub CLI (gh) is not installed — install it first (macOS: brew install gh)")
	}
	v, err := Version()
	if err != nil {
		return err
	}
	if less(v, MinVersion) {
		return fmt.Errorf("gh %d.%d.%d is too old — multi-account support needs >= %d.%d.%d (brew upgrade gh)",
			v[0], v[1], v[2], MinVersion[0], MinVersion[1], MinVersion[2])
	}
	return nil
}

func less(a, b [3]int) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// run executes gh with inherited stdio (so interactive flows like
// `gh auth login` work) and returns any error.
func run(args ...string) error {
	cmd := exec.Command(Bin(), args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// AuthSwitch makes user the active gh account.
func AuthSwitch(user string) error {
	if err := run("auth", "switch", "--hostname", DefaultHost, "--user", user); err != nil {
		return fmt.Errorf("gh auth switch failed (is %q logged in? try `ghs add`): %w", user, err)
	}
	return nil
}

// AuthLogin runs the interactive gh login flow.
func AuthLogin() error {
	return run("auth", "login", "--hostname", DefaultHost)
}

// AuthLogout removes a gh account.
func AuthLogout(user string) error {
	return run("auth", "logout", "--hostname", DefaultHost, "--user", user)
}

// SetupGit configures gh as git's credential helper for github.com.
func SetupGit() error {
	if err := run("auth", "setup-git", "--hostname", DefaultHost); err != nil {
		return fmt.Errorf("gh auth setup-git failed: %w", err)
	}
	return nil
}
