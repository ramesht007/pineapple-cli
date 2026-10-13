// Package paths resolves every filesystem location pine touches.
// PINEAPPLE_HOME and CLAUDE_DIR override the defaults (used by tests).
package paths

import (
	"os"
	"path/filepath"
)

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func PineappleHome() string { return env("PINEAPPLE_HOME", filepath.Join(home(), ".pineapple")) }
func ClaudeDir() string     { return env("CLAUDE_DIR", filepath.Join(home(), ".claude")) }

func ConfigFile() string   { return filepath.Join(PineappleHome(), "config.json") }
func StateFile() string    { return filepath.Join(PineappleHome(), "state.json") }
func RepoDir() string      { return filepath.Join(PineappleHome(), "repo") }
func BackupsDir() string   { return filepath.Join(PineappleHome(), "backups") }
func ClaudeSkills() string { return filepath.Join(ClaudeDir(), "skills") }
func ClaudeMods() string   { return filepath.Join(ClaudeDir(), "mods") }
