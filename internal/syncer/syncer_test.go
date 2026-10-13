package syncer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rel = "skills/a/SKILL.md"

func sp(s string) *string { return &s }

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func write(t *testing.T, p string, s *string) {
	t.Helper()
	if s == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(*s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyMatrix(t *testing.T) {
	// nil = file absent. state is the content at last sync.
	tests := []struct {
		name                string
		remote, local, last *string
		wantLocal           *string
		wantApplied         bool
		wantBackup          *string // content expected in backup (nil = conflict not expected)
	}{
		{"remote changed, local untouched", sp("v2"), sp("v1"), sp("v1"), sp("v2"), true, nil},
		{"remote changed, local modified", sp("v2"), sp("mine"), sp("v1"), sp("v2"), true, sp("mine")},
		{"both changed identically", sp("v2"), sp("v2"), sp("v1"), sp("v2"), false, nil},
		{"remote unchanged, local modified", sp("v1"), sp("mine"), sp("v1"), sp("mine"), false, nil},
		{"remote new, no local", sp("v1"), nil, nil, sp("v1"), true, nil},
		{"remote new, local pre-existing differs", sp("v1"), sp("mine"), nil, sp("v1"), true, sp("mine")},
		{"remote deleted, local untouched", nil, sp("v1"), sp("v1"), nil, true, nil},
		{"remote deleted, local modified", nil, sp("mine"), sp("v1"), nil, true, sp("mine")},
		{"remote deleted, local already gone", nil, nil, sp("v1"), nil, false, nil},
		{"local new, not in repo or state", nil, sp("new"), nil, sp("new"), false, nil},
		{"remote changed, local deleted", sp("v2"), nil, sp("v1"), sp("v2"), true, nil},
		{"remote unchanged, local deleted", sp("v1"), nil, sp("v1"), nil, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("PINEAPPLE_HOME", filepath.Join(tmp, "home"))
			repoDir, claude := filepath.Join(tmp, "repo"), filepath.Join(tmp, "claude")
			skills, mods := filepath.Join(claude, "skills"), filepath.Join(claude, "mods")
			write(t, filepath.Join(repoDir, filepath.FromSlash(rel)), tt.remote)
			write(t, filepath.Join(claude, filepath.FromSlash(rel)), tt.local)
			state := map[string]string{}
			if tt.last != nil {
				state[rel] = sum(*tt.last)
			}

			applied, conflicts, err := Apply(repoDir, skills, mods, state)
			if err != nil {
				t.Fatal(err)
			}
			if (len(applied) > 0) != tt.wantApplied {
				t.Errorf("applied=%v want applied=%v", applied, tt.wantApplied)
			}
			got, rerr := os.ReadFile(filepath.Join(claude, filepath.FromSlash(rel)))
			switch {
			case tt.wantLocal == nil && rerr == nil:
				t.Errorf("local should be absent, has %q", got)
			case tt.wantLocal != nil && string(got) != *tt.wantLocal:
				t.Errorf("local=%q (err %v) want %q", got, rerr, *tt.wantLocal)
			}
			if (len(conflicts) > 0) != (tt.wantBackup != nil) {
				t.Fatalf("conflicts=%v wantBackup=%v", conflicts, tt.wantBackup)
			}
			if tt.wantBackup != nil {
				_, bp, _ := strings.Cut(conflicts[0], " -> ")
				if b, err := os.ReadFile(bp); err != nil || string(b) != *tt.wantBackup {
					t.Errorf("backup %s = %q (err %v), want %q", bp, b, err, *tt.wantBackup)
				}
			}
		})
	}
}

func TestSnapshotMirror(t *testing.T) {
	tmp := t.TempDir()
	skills, mods, repoDir := filepath.Join(tmp, "skills"), filepath.Join(tmp, "mods"), filepath.Join(tmp, "repo")
	write(t, filepath.Join(skills, "a", "SKILL.md"), sp("one"))
	write(t, filepath.Join(skills, "synced", "x.md"), sp("managed"))
	write(t, filepath.Join(skills, "a", ".DS_Store"), sp("junk"))
	write(t, filepath.Join(mods, "m", "plugin.json"), sp("{}"))
	write(t, filepath.Join(repoDir, "skills", "stale", "old.md"), sp("old"))
	if err := os.Symlink(filepath.Join(skills, "a", "SKILL.md"), filepath.Join(skills, "a", "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(skills, "a", "SKILL.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Snapshot(skills, mods, repoDir); err != nil {
		t.Fatal(err)
	}
	got, err := Rehash(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"skills/a/SKILL.md": sum("one"), "mods/m/plugin.json": sum("{}")}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %s want %s", k, got[k], v)
		}
	}
	if fi, _ := os.Stat(filepath.Join(repoDir, "skills", "a", "SKILL.md")); fi == nil || fi.Mode().Perm()&0o100 == 0 {
		t.Error("exec bit lost")
	}
	if _, err := os.Stat(filepath.Join(repoDir, "skills", "stale")); err == nil {
		t.Error("stale dir not pruned")
	}
}
