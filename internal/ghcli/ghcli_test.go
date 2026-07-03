package ghcli

import (
	"os"
	"path/filepath"
	"testing"
)

func writeHosts(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GH_CONFIG_DIR", dir)
}

func TestReadHostsKeyring(t *testing.T) {
	// The exact on-disk shape of a keyring-token login (verified live).
	writeHosts(t, `github.com:
    git_protocol: https
    users:
        octocat:
    user: octocat
`)
	h, err := ReadHosts()
	if err != nil {
		t.Fatal(err)
	}
	if h.Active != "octocat" {
		t.Errorf("active = %q, want octocat", h.Active)
	}
	if len(h.Users) != 1 || h.Users[0] != "octocat" {
		t.Errorf("users = %v", h.Users)
	}
}

func TestReadHostsMultiUserInsecure(t *testing.T) {
	writeHosts(t, `github.com:
    git_protocol: https
    users:
        alice:
            oauth_token: gho_aaa
        bob:
            oauth_token: gho_bbb
    user: bob
    oauth_token: gho_bbb
`)
	h, err := ReadHosts()
	if err != nil {
		t.Fatal(err)
	}
	if h.Active != "bob" {
		t.Errorf("active = %q, want bob", h.Active)
	}
	if len(h.Users) != 2 {
		t.Errorf("users = %v, want 2", h.Users)
	}
}

func TestReadHostsMultiHost(t *testing.T) {
	writeHosts(t, `github.com:
    users:
        alice:
    user: alice
ghe.example.com:
    users:
        corp-alice:
    user: corp-alice
`)
	h, err := ReadHosts()
	if err != nil {
		t.Fatal(err)
	}
	if h.Active != "alice" {
		t.Errorf("active = %q, want alice (github.com only)", h.Active)
	}
}

func TestReadHostsNoActiveUser(t *testing.T) {
	writeHosts(t, `github.com:
    users:
        alice:
`)
	h, err := ReadHosts()
	if err != nil {
		t.Fatal(err)
	}
	if h.Active != "" {
		t.Errorf("active = %q, want empty", h.Active)
	}
	if len(h.Users) != 1 {
		t.Errorf("users = %v", h.Users)
	}
}

func TestReadHostsMissingFile(t *testing.T) {
	writeHosts(t, "")
	h, err := ReadHosts()
	if err != nil {
		t.Fatal(err)
	}
	if h.Active != "" || len(h.Users) != 0 {
		t.Errorf("expected empty hosts, got %+v", h)
	}
}

func TestVersionGate(t *testing.T) {
	if less([3]int{2, 86, 0}, MinVersion) {
		t.Error("2.86.0 should pass the gate")
	}
	if !less([3]int{2, 39, 9}, MinVersion) {
		t.Error("2.39.9 should fail the gate")
	}
	if less([3]int{3, 0, 0}, MinVersion) {
		t.Error("3.0.0 should pass the gate")
	}
}
