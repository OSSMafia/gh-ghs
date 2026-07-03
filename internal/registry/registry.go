// Package registry owns the ghs TOML config: profiles, directory pins, and
// lifecycle state. It is the single source of truth; all gitconfig fragments
// are regenerated from it wholesale.
package registry

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/OSSMafia/gh-ghs/internal/paths"
)

// Profile maps a friendly name to a GitHub account and git identity.
type Profile struct {
	Username string `toml:"username"` // gh account login
	Name     string `toml:"name"`     // git user.name
	Email    string `toml:"email"`    // git user.email
}

// Pin binds a directory tree to a profile.
type Pin struct {
	Profile string `toml:"profile"`
	Path    string `toml:"path"` // canonical, no trailing slash
}

// State tracks everything ghs has changed on the machine so uninstall can
// revert it all.
type State struct {
	Symlinks        []string `toml:"symlinks,omitempty"`
	SetupGitRan     bool     `toml:"setup_git_ran,omitempty"`
	GuardRepos      []string `toml:"guard_repos,omitempty"`
	ZshrcSnippet    bool     `toml:"zshrc_snippet_installed,omitempty"`
	ClaudeHookFiles []string `toml:"claude_hook_files,omitempty"`
	CursorRuleFiles []string `toml:"cursor_rule_files,omitempty"`
	FirstRunBackup  string   `toml:"first_run_backup,omitempty"`
	IncludeAdded    bool     `toml:"include_added,omitempty"`
}

// Config is the on-disk registry.
type Config struct {
	Version  int                `toml:"version"`
	Profiles map[string]Profile `toml:"profiles"`
	Pins     []Pin              `toml:"pins,omitempty"`
	State    State              `toml:"state,omitempty"`
}

// Load reads the registry, returning an empty one if the file is absent.
func Load() (*Config, error) {
	file, err := paths.ConfigFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return &Config{Version: 1, Profiles: map[string]Profile{}}, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	return cfg, nil
}

// Save writes the registry atomically.
func (c *Config) Save() error {
	file, err := paths.ConfigFile()
	if err != nil {
		return err
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return paths.WriteFileAtomic(file, data, 0o600)
}

// ProfileNames returns profile names sorted alphabetically.
func (c *Config) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for n := range c.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ProfileForUsername finds the profile matching a gh account login.
func (c *Config) ProfileForUsername(username string) (string, Profile, bool) {
	for _, name := range c.ProfileNames() {
		if strings.EqualFold(c.Profiles[name].Username, username) {
			return name, c.Profiles[name], true
		}
	}
	return "", Profile{}, false
}

// ProfileForEmail finds the profile matching a git user.email.
func (c *Config) ProfileForEmail(email string) (string, Profile, bool) {
	for _, name := range c.ProfileNames() {
		if strings.EqualFold(c.Profiles[name].Email, email) {
			return name, c.Profiles[name], true
		}
	}
	return "", Profile{}, false
}

// PinFor returns the deepest pin whose path contains dir (dir must be
// canonical), or nil.
func (c *Config) PinFor(dir string) *Pin {
	var best *Pin
	for i := range c.Pins {
		p := &c.Pins[i]
		if dir == p.Path || strings.HasPrefix(dir, p.Path+string(os.PathSeparator)) {
			if best == nil || len(p.Path) > len(best.Path) {
				best = p
			}
		}
	}
	return best
}

// SetPin adds or replaces the pin for a canonical path.
func (c *Config) SetPin(profile, path string) {
	for i := range c.Pins {
		if c.Pins[i].Path == path {
			c.Pins[i].Profile = profile
			return
		}
	}
	c.Pins = append(c.Pins, Pin{Profile: profile, Path: path})
}

// RemovePin removes the pin with an exact canonical path. Reports whether a
// pin was removed.
func (c *Config) RemovePin(path string) bool {
	for i := range c.Pins {
		if c.Pins[i].Path == path {
			c.Pins = append(c.Pins[:i], c.Pins[i+1:]...)
			return true
		}
	}
	return false
}

// RemovePinsForProfile drops all pins referencing a profile.
func (c *Config) RemovePinsForProfile(profile string) {
	kept := c.Pins[:0]
	for _, p := range c.Pins {
		if p.Profile != profile {
			kept = append(kept, p)
		}
	}
	c.Pins = kept
}

// PinsSortedByDepth returns pins ordered shallow-to-deep so that when they
// are rendered as includeIf blocks, deeper (more specific) pins win under
// git's last-include-wins rule.
func (c *Config) PinsSortedByDepth() []Pin {
	pins := make([]Pin, len(c.Pins))
	copy(pins, c.Pins)
	sort.Slice(pins, func(i, j int) bool {
		di := strings.Count(pins[i].Path, string(os.PathSeparator))
		dj := strings.Count(pins[j].Path, string(os.PathSeparator))
		if di != dj {
			return di < dj
		}
		return pins[i].Path < pins[j].Path
	})
	return pins
}

// PinnedProfiles returns the set of profile names that have at least one pin.
func (c *Config) PinnedProfiles() map[string]bool {
	out := map[string]bool{}
	for _, p := range c.Pins {
		out[p.Profile] = true
	}
	return out
}
