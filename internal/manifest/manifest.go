// Package manifest exports/applies the Claude Code plugin set and an
// allowlist of settings.json keys.
package manifest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ramesht007/pineapple-cli/internal/config"
	"github.com/ramesht007/pineapple-cli/internal/paths"
)

const placeholder = "${CLAUDE_DIR}"

var allowlist = []string{"enabledPlugins", "extraKnownMarketplaces", "theme", "statusLine"}

type Plugin struct {
	ID          string `json:"id"`
	Marketplace string `json:"marketplace"`
	Version     string `json:"version"`
	SHA         string `json:"sha"`
	Enabled     bool   `json:"enabled"`
}

type Manifest struct {
	Schema       int                          `json:"schema"`
	Marketplaces map[string]map[string]string `json:"marketplaces"`
	Plugins      []Plugin                     `json:"plugins"`
}

type installedEntry struct {
	Scope   string `json:"scope"`
	Version string `json:"version"`
	SHA     string `json:"gitCommitSha"`
}

// readJSON reads path into v; a missing file leaves v untouched.
func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// toPortable maps a path under claudeDir to ${CLAUDE_DIR}/rel.
func toPortable(p, claudeDir string) (string, bool) {
	rel, err := filepath.Rel(claudeDir, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	if rel == "." {
		return placeholder, true
	}
	return placeholder + "/" + filepath.ToSlash(rel), true
}

func fromPortable(s, claudeDir string) string {
	if !strings.HasPrefix(s, placeholder) {
		return s
	}
	return filepath.Join(claudeDir, filepath.FromSlash(strings.TrimPrefix(s, placeholder)))
}

// installed returns id -> chosen entry (scope "user" preferred, else first).
func installed() (map[string]installedEntry, error) {
	var f struct {
		Plugins map[string][]installedEntry `json:"plugins"`
	}
	if err := readJSON(paths.InstalledPlugins(), &f); err != nil {
		return nil, err
	}
	out := map[string]installedEntry{}
	for id, es := range f.Plugins {
		if len(es) == 0 {
			continue
		}
		e := es[0]
		for _, c := range es {
			if c.Scope == "user" {
				e = c
				break
			}
		}
		out[id] = e
	}
	return out, nil
}

func Push(repoDir string) error {
	claudeDir := paths.ClaudeDir()
	inst, err := installed()
	if err != nil {
		return err
	}
	var known map[string]struct {
		Source map[string]any `json:"source"`
	}
	if err := readJSON(paths.KnownMarketplaces(), &known); err != nil {
		return err
	}
	settings := map[string]json.RawMessage{}
	if err := readJSON(paths.ClaudeSettings(), &settings); err != nil {
		return err
	}

	m := Manifest{Schema: 1, Marketplaces: map[string]map[string]string{}, Plugins: []Plugin{}}
	for name, k := range known {
		typ, _ := k.Source["source"].(string)
		var val string
		for _, key := range []string{"repo", "url", "path"} {
			if s, ok := k.Source[key].(string); ok {
				val = s
				break
			}
		}
		if typ == "" || val == "" {
			continue
		}
		if typ == "directory" {
			p, ok := toPortable(val, claudeDir)
			if !ok {
				fmt.Fprintf(os.Stderr, "warning: marketplace %s: directory %s is outside %s, not portable; skipped\n", name, val, claudeDir)
				continue
			}
			val = p
		}
		m.Marketplaces[name] = map[string]string{typ: val}
	}

	enabled := map[string]bool{}
	if raw, ok := settings["enabledPlugins"]; ok {
		_ = json.Unmarshal(raw, &enabled)
	}
	ids := map[string]bool{}
	for id := range inst {
		ids[id] = true
	}
	for id := range enabled {
		ids[id] = true
	}
	for id := range ids {
		e := inst[id]
		mk := ""
		if i := strings.LastIndex(id, "@"); i >= 0 {
			mk = id[i+1:]
		}
		m.Plugins = append(m.Plugins, Plugin{ID: id, Marketplace: mk, Version: e.Version, SHA: e.SHA, Enabled: enabled[id]})
	}
	sort.Slice(m.Plugins, func(i, j int) bool { return m.Plugins[i].ID < m.Plugins[j].ID })
	if err := config.WriteJSON(filepath.Join(repoDir, "manifest.json"), m, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	sync := map[string]any{}
	for _, k := range allowlist {
		raw, ok := settings[k]
		if !ok {
			continue
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("parse settings key %s: %w", k, err)
		}
		if k == "extraKnownMarketplaces" {
			v = portableMarketplaces(v, claudeDir)
		}
		sync[k] = v
	}
	if err := config.WriteJSON(filepath.Join(repoDir, "settings.sync.json"), sync, 0o644); err != nil {
		return fmt.Errorf("write settings.sync: %w", err)
	}
	return nil
}

func portableMarketplaces(v any, claudeDir string) any {
	mm, ok := v.(map[string]any)
	if !ok {
		return v
	}
	for name, e := range mm {
		em, _ := e.(map[string]any)
		src, _ := em["source"].(map[string]any)
		if src == nil || src["source"] != "directory" {
			continue
		}
		p, _ := src["path"].(string)
		pp, ok := toPortable(p, claudeDir)
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: extraKnownMarketplaces.%s: directory %s is outside %s, not portable; skipped\n", name, p, claudeDir)
			delete(mm, name)
			continue
		}
		src["path"] = pp
	}
	return mm
}

func expand(v any, claudeDir string) any {
	switch t := v.(type) {
	case string:
		return fromPortable(t, claudeDir)
	case map[string]any:
		for k, x := range t {
			t[k] = expand(x, claudeDir)
		}
	case []any:
		for i, x := range t {
			t[i] = expand(x, claudeDir)
		}
	}
	return v
}

// Apply merges settings and installs plugins. Plugin problems are warnings only.
func Apply(repoDir string) error {
	claudeDir := paths.ClaudeDir()
	if err := applySettings(repoDir, claudeDir); err != nil {
		return err
	}
	for _, w := range applyPlugins(repoDir, claudeDir) {
		fmt.Fprintln(os.Stderr, w)
	}
	return nil
}

func applySettings(repoDir, claudeDir string) error {
	var sync map[string]any
	src := filepath.Join(repoDir, "settings.sync.json")
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	if err := readJSON(src, &sync); err != nil {
		return err
	}
	settings := map[string]json.RawMessage{}
	sp := paths.ClaudeSettings()
	if old, err := os.ReadFile(sp); err == nil {
		if err := json.Unmarshal(old, &settings); err != nil {
			return fmt.Errorf("parse %s: %w", sp, err)
		}
		if err := os.WriteFile(sp+".pineapple-bak", old, 0o644); err != nil {
			return fmt.Errorf("backup settings: %w", err)
		}
	}
	for _, k := range allowlist {
		v, ok := sync[k]
		if !ok {
			continue
		}
		b, err := json.Marshal(expand(v, claudeDir))
		if err != nil {
			return err
		}
		settings[k] = b
	}
	if err := config.WriteJSON(sp, settings, 0o644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}

func applyPlugins(repoDir, claudeDir string) []string {
	var warns []string
	var m Manifest
	if err := readJSON(filepath.Join(repoDir, "manifest.json"), &m); err != nil {
		return []string{"warning: " + err.Error()}
	}
	inst, err := installed()
	if err != nil {
		return []string{"warning: " + err.Error()}
	}
	_, noClaude := os.LookupEnv("PINEAPPLE_NO_CLAUDE")
	if !noClaude {
		if _, err := exec.LookPath("claude"); err != nil {
			noClaude = true
		}
	}
	var todo []Plugin
	for _, p := range m.Plugins {
		// builtin plugins ship with Claude Code; the settings merge enables them
		if !p.Enabled || p.Marketplace == "builtin" || (p.Version != "" && inst[p.ID].Version == p.Version) {
			continue
		}
		todo = append(todo, p)
	}
	if noClaude {
		for _, p := range todo {
			fmt.Printf("plugin %s will be installed by Claude Code on next launch\n", p.ID)
		}
		return warns
	}
	warns = append(warns, refreshMarketplaces(m, todo)...)
	for _, p := range todo {
		restore := func() {}
		if clone := filepath.Join(paths.ClaudePlugins(), "marketplaces", p.Marketplace); p.SHA != "" && p.Marketplace != "" && isDir(clone) {
			r, err := pinClone(clone, p.SHA)
			if err != nil {
				warns = append(warns, fmt.Sprintf("warning: pin %s to %s failed: %v", p.Marketplace, p.SHA, err))
			}
			restore = r
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		out, err := exec.CommandContext(ctx, "claude", "plugin", "install", p.ID).CombinedOutput()
		cancel()
		restore() // install copied files into the plugin cache
		if err != nil {
			warns = append(warns, fmt.Sprintf("warning: claude plugin install %s failed: %v: %s", p.ID, err, tail(out)))
		}
	}
	if len(todo) == 0 {
		return warns
	}
	inst, err = installed()
	if err != nil {
		return append(warns, "warning: "+err.Error())
	}
	for _, p := range todo {
		if p.Version != "" && inst[p.ID].Version != p.Version {
			warns = append(warns, "warning: drift: "+driftLine(p.ID, p.Version, inst[p.ID].Version))
		}
	}
	return warns
}

func driftLine(id, want, got string) string {
	if got == "" {
		return fmt.Sprintf("%s wanted %s: not installed", id, want)
	}
	return fmt.Sprintf("%s wanted %s, got %q", id, want, got)
}

// refreshMarketplaces adds missing github marketplaces and updates existing
// git clones for the marketplaces todo needs. Failures are warnings.
func refreshMarketplaces(m Manifest, todo []Plugin) []string {
	var warns []string
	seen := map[string]bool{}
	for _, p := range todo {
		name := p.Marketplace
		if name == "" || name == "builtin" || seen[name] {
			continue
		}
		seen[name] = true
		clone := filepath.Join(paths.ClaudePlugins(), "marketplaces", name)
		var args []string
		switch {
		case !isDir(clone) && m.Marketplaces[name]["github"] != "":
			args = []string{"plugin", "marketplace", "add", m.Marketplaces[name]["github"]}
		case isDir(filepath.Join(clone, ".git")):
			args = []string{"plugin", "marketplace", "update", name}
		default: // directory-source or unknown: nothing to refresh
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		out, err := exec.CommandContext(ctx, "claude", args...).CombinedOutput()
		cancel()
		if err != nil {
			warns = append(warns, fmt.Sprintf("warning: claude %s failed: %v: %s", strings.Join(args, " "), err, tail(out)))
		}
	}
	return warns
}

// pinClone checks clone out at sha (fetching it, unshallowing if needed) and
// returns a func restoring the original ref. restore is always non-nil.
func pinClone(clone, sha string) (restore func(), err error) {
	git := func(args ...string) ([]byte, error) {
		return exec.Command("git", append([]string{"-C", clone}, args...)...).CombinedOutput()
	}
	orig, e := git("symbolic-ref", "--short", "-q", "HEAD")
	if e != nil {
		orig, _ = git("rev-parse", "HEAD")
	}
	ref := strings.TrimSpace(string(orig))
	restore = func() {
		if ref != "" {
			_, _ = git("checkout", "--quiet", ref)
		}
	}
	if _, e := git("fetch", "--quiet", "origin", sha); e != nil {
		_, _ = git("fetch", "--quiet", "--unshallow", "origin")
		_, _ = git("fetch", "--quiet", "origin")
	}
	if out, e := git("checkout", "--quiet", sha); e != nil {
		return restore, errors.New(tail(out))
	}
	return restore, nil
}

func isDir(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }

func tail(b []byte) string {
	b = bytes.TrimSpace(b)
	if len(b) > 400 {
		b = b[len(b)-400:]
	}
	return string(b)
}
