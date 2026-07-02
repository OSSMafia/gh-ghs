// Package paths centralizes filesystem locations for ghs-managed state.
// All lookups honor environment overrides (HOME, XDG_CONFIG_HOME,
// GIT_CONFIG_GLOBAL) so tests can sandbox every write.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// Home returns the user's home directory.
func Home() (string, error) {
	return os.UserHomeDir()
}

// ConfigDir returns the ghs configuration directory
// ($XDG_CONFIG_HOME/ghs or ~/.config/ghs).
func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ghs"), nil
	}
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ghs"), nil
}

// ConfigFile returns the path to the ghs TOML registry.
func ConfigFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// GhsGitconfig returns the path of the ghs-managed gitconfig included from
// the user's global gitconfig.
func GhsGitconfig() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ghs.gitconfig"), nil
}

// ProfilesDir returns the directory holding per-profile gitconfig fragments.
func ProfilesDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles"), nil
}

// ProfileFragment returns the gitconfig fragment path for a profile.
func ProfileFragment(profile string) (string, error) {
	dir, err := ProfilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, profile+".gitconfig"), nil
}

// BackupDir returns the directory where ghs stores backups.
func BackupDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "backup"), nil
}

// GlobalGitconfig returns the file `git config --global` writes to:
// $GIT_CONFIG_GLOBAL if set, else ~/.gitconfig. (git also reads
// ~/.config/git/config, but only writes there when ~/.gitconfig is absent;
// for backup purposes the write target is what matters.)
func GlobalGitconfig() (string, error) {
	if p := os.Getenv("GIT_CONFIG_GLOBAL"); p != "" {
		return p, nil
	}
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gitconfig"), nil
}

// DefaultBinDir is where `ghs link` places symlinks.
func DefaultBinDir() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}

// Canonicalize resolves a path to its absolute, symlink-free form. The path
// must exist; git matches `includeIf "gitdir:"` patterns against resolved
// paths, so pins must be canonical (e.g. /tmp -> /private/tmp on macOS).
func Canonicalize(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %s: %w", p, err)
	}
	return resolved, nil
}

// Self returns the resolved path of the running binary (the real extension
// binary, even when invoked through a ghs/git-ghs symlink).
func Self() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return resolved, nil
}

// WriteFileAtomic writes data to path via a temp file + rename in the same
// directory, creating parent directories as needed.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".ghs-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
