package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) (claude, repo string) {
	claude, repo = t.TempDir(), t.TempDir()
	t.Setenv("CLAUDE_DIR", claude)
	t.Setenv("PINEAPPLE_HOME", t.TempDir())
	t.Setenv("PINEAPPLE_NO_CLAUDE", "1")
	return
}

func load(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func fixture(t *testing.T, c string) {
	write(t, filepath.Join(c, "plugins/installed_plugins.json"), `{"version":2,"plugins":{
"ponytail@ponytail":[{"scope":"user","version":"4.8.4","gitCommitSha":"16f2"}],
"nosha@ponytail":[{"scope":"user","version":"1.0.0"}]}}`)
	write(t, filepath.Join(c, "plugins/known_marketplaces.json"), `{
"ponytail":{"source":{"source":"github","repo":"DietrichGebert/ponytail"}},
"local-mods":{"source":{"source":"directory","path":"`+c+`/mods"}},
"outside":{"source":{"source":"directory","path":"/elsewhere/x"}}}`)
	write(t, filepath.Join(c, "settings.json"), `{"env":{"A":"1"},"model":"opus","theme":"dark",
"enabledPlugins":{"ponytail@ponytail":true,"nosha@ponytail":false,"ghost@x":true},
"extraKnownMarketplaces":{"local-mods":{"source":{"source":"directory","path":"`+c+`/mods"}}}}`)
}

func TestPush(t *testing.T) {
	c, r := setup(t)
	fixture(t, c)
	if err := Push(r); err != nil {
		t.Fatal(err)
	}
	m := load(t, filepath.Join(r, "manifest.json"))
	mk := m["marketplaces"].(map[string]any)
	if mk["local-mods"].(map[string]any)["directory"] != "${CLAUDE_DIR}/mods" {
		t.Errorf("rewrite: %v", mk)
	}
	if _, ok := mk["outside"]; ok {
		t.Error("outside marketplace not skipped")
	}
	if mk["ponytail"].(map[string]any)["github"] != "DietrichGebert/ponytail" {
		t.Errorf("github: %v", mk)
	}
	ps := m["plugins"].([]any)
	if len(ps) != 3 {
		t.Fatalf("plugins: %v", ps)
	}
	// sorted: ghost, nosha, ponytail
	if g := ps[0].(map[string]any); g["id"] != "ghost@x" || g["version"] != "" || g["enabled"] != true {
		t.Errorf("ghost: %v", g)
	}
	if n := ps[1].(map[string]any); n["enabled"] != false {
		t.Errorf("nosha: %v", n)
	}
	if p := ps[2].(map[string]any); p["sha"] != "16f2" || p["version"] != "4.8.4" || p["enabled"] != true {
		t.Errorf("ponytail: %v", p)
	}
	s := load(t, filepath.Join(r, "settings.sync.json"))
	if _, ok := s["env"]; ok || s["model"] != nil {
		t.Error("non-allowlist leaked")
	}
	if s["theme"] != "dark" {
		t.Error("theme")
	}
	p := s["extraKnownMarketplaces"].(map[string]any)["local-mods"].(map[string]any)["source"].(map[string]any)["path"]
	if p != "${CLAUDE_DIR}/mods" {
		t.Errorf("settings rewrite: %v", p)
	}
}

func TestPushEmpty(t *testing.T) {
	_, r := setup(t)
	if err := Push(r); err != nil {
		t.Fatal(err)
	}
	m := load(t, filepath.Join(r, "manifest.json"))
	if len(m["plugins"].([]any)) != 0 || len(m["marketplaces"].(map[string]any)) != 0 {
		t.Errorf("not empty: %v", m)
	}
}

func TestRoundTripStable(t *testing.T) {
	c, r := setup(t)
	fixture(t, c)
	var first [2][]byte
	for i := 0; i < 2; i++ {
		if err := Push(r); err != nil {
			t.Fatal(err)
		}
		a, _ := os.ReadFile(filepath.Join(r, "manifest.json"))
		b, _ := os.ReadFile(filepath.Join(r, "settings.sync.json"))
		if i == 0 {
			first = [2][]byte{a, b}
		} else if string(a) != string(first[0]) || string(b) != string(first[1]) {
			t.Error("push not stable")
		}
	}
}

func TestApplySettingsAndManifest(t *testing.T) {
	c, r := setup(t)
	write(t, filepath.Join(c, "settings.json"), `{"env":{"A":"1"},"model":"opus","enabledPlugins":{"old@x":true}}`)
	write(t, filepath.Join(r, "settings.sync.json"), `{"enabledPlugins":{"new@x":true},"theme":"light",
"extraKnownMarketplaces":{"m":{"source":{"source":"directory","path":"${CLAUDE_DIR}/mods"}}}}`)
	write(t, filepath.Join(r, "manifest.json"), `{"schema":1,"plugins":[{"id":"new@x","marketplace":"x","version":"1","sha":"abc","enabled":true}]}`)
	if err := Apply(r); err != nil {
		t.Fatal(err)
	}
	s := load(t, filepath.Join(c, "settings.json"))
	if s["model"] != "opus" || s["env"].(map[string]any)["A"] != "1" || s["theme"] != "light" {
		t.Errorf("merge: %v", s)
	}
	ep := s["enabledPlugins"].(map[string]any)
	if _, old := ep["old@x"]; old || ep["new@x"] != true {
		t.Errorf("enabledPlugins: %v", ep)
	}
	p := s["extraKnownMarketplaces"].(map[string]any)["m"].(map[string]any)["source"].(map[string]any)["path"]
	if p != filepath.Join(c, "mods") {
		t.Errorf("expand: %v", p)
	}
	if _, err := os.Stat(filepath.Join(c, "settings.json.pineapple-bak")); err != nil {
		t.Error("no backup")
	}
}

func TestApplyNothing(t *testing.T) {
	_, r := setup(t)
	if err := Apply(r); err != nil {
		t.Fatal(err)
	}
}
