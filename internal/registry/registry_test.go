package registry

import (
	"testing"
)

func sandbox(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestRoundTrip(t *testing.T) {
	sandbox(t)
	c := &Config{Version: 1, Profiles: map[string]Profile{
		"work": {Username: "octo-work", Name: "Mona Octocat", Email: "dev@company.com"},
	}}
	c.SetPin("work", "/home/x/work")
	c.State.SetupGitRan = true
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Profiles["work"].Email != "dev@company.com" {
		t.Errorf("profile lost: %+v", got.Profiles)
	}
	if len(got.Pins) != 1 || got.Pins[0].Path != "/home/x/work" {
		t.Errorf("pins lost: %+v", got.Pins)
	}
	if !got.State.SetupGitRan {
		t.Error("state lost")
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	sandbox(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Profiles) != 0 || len(c.Pins) != 0 {
		t.Errorf("expected empty config, got %+v", c)
	}
}

func TestPinForLongestPrefix(t *testing.T) {
	c := &Config{Profiles: map[string]Profile{}}
	c.SetPin("work", "/u/work")
	c.SetPin("client", "/u/work/client-x")

	cases := []struct{ dir, want string }{
		{"/u/work/repo", "work"},
		{"/u/work/client-x/repo", "client"},
		{"/u/work/client-x", "client"},
		{"/u/work", "work"},
		{"/u/workother", ""}, // prefix must respect path boundaries
		{"/u/personal", ""},
	}
	for _, tc := range cases {
		got := ""
		if p := c.PinFor(tc.dir); p != nil {
			got = p.Profile
		}
		if got != tc.want {
			t.Errorf("PinFor(%s) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

func TestPinsSortedByDepth(t *testing.T) {
	c := &Config{}
	c.SetPin("deep", "/a/b/c")
	c.SetPin("shallow", "/a")
	c.SetPin("mid", "/a/b")
	pins := c.PinsSortedByDepth()
	if pins[0].Profile != "shallow" || pins[2].Profile != "deep" {
		t.Errorf("wrong order: %+v", pins)
	}
}

func TestSetPinReplaces(t *testing.T) {
	c := &Config{}
	c.SetPin("a", "/x")
	c.SetPin("b", "/x")
	if len(c.Pins) != 1 || c.Pins[0].Profile != "b" {
		t.Errorf("expected replacement, got %+v", c.Pins)
	}
}

func TestRemovePinsForProfile(t *testing.T) {
	c := &Config{}
	c.SetPin("a", "/x")
	c.SetPin("b", "/y")
	c.SetPin("a", "/z")
	c.RemovePinsForProfile("a")
	if len(c.Pins) != 1 || c.Pins[0].Profile != "b" {
		t.Errorf("got %+v", c.Pins)
	}
}
