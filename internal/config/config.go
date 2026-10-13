// Package config loads and saves ~/.pineapple/config.json and state.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ramesht007/pineapple-cli/internal/paths"
)

type Config struct {
	Repo                string `json:"repo"`    // git URL or owner/name
	Machine             string `json:"machine"` // default os.Hostname()
	PullIntervalMinutes int    `json:"pullIntervalMinutes"`
	PushIntervalHours   int    `json:"pushIntervalHours"`
}

type State struct {
	Files    map[string]string `json:"files"` // relpath -> sha256 at last successful sync
	LastPush time.Time         `json:"lastPush"`
	LastPull time.Time         `json:"lastPull"`
}

// New returns a Config for repo with defaults filled in.
func New(repo string) Config {
	c := Config{Repo: repo}
	c.defaults()
	return c
}

func (c *Config) defaults() {
	if c.Machine == "" {
		c.Machine, _ = os.Hostname()
		if c.Machine == "" {
			c.Machine = "unknown"
		}
	}
	if c.PullIntervalMinutes == 0 {
		c.PullIntervalMinutes = 15
	}
	if c.PushIntervalHours == 0 {
		c.PushIntervalHours = 24
	}
}

func Exists() bool {
	_, err := os.Stat(paths.ConfigFile())
	return err == nil
}

func Load() (Config, error) {
	var c Config
	b, err := os.ReadFile(paths.ConfigFile())
	if errors.Is(err, os.ErrNotExist) {
		return c, fmt.Errorf("not initialised: run `pine init` first")
	}
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", paths.ConfigFile(), err)
	}
	c.defaults()
	return c, nil
}

func Save(c Config) error { return WriteJSON(paths.ConfigFile(), c, 0o600) }

func LoadState() (State, error) {
	s := State{Files: map[string]string{}}
	b, err := os.ReadFile(paths.StateFile())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("parse %s: %w", paths.StateFile(), err)
	}
	if s.Files == nil {
		s.Files = map[string]string{}
	}
	return s, nil
}

func SaveState(s State) error { return WriteJSON(paths.StateFile(), s, 0o600) }

// WriteJSON writes v as indented JSON atomically (temp file + rename).
func WriteJSON(path string, v any, perm os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
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
	return os.Rename(tmp.Name(), path)
}
