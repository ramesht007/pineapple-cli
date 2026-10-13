package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramesht007/pineapple-cli/internal/paths"
)

// sandbox points PINEAPPLE_HOME and CLAUDE_DIR at fresh temp dirs and returns a repo dir.
func sandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("PINEAPPLE_HOME", filepath.Join(root, "home"))
	t.Setenv("CLAUDE_DIR", filepath.Join(root, "claude"))
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestEnsureKey(t *testing.T) {
	sandbox(t)
	pub, created, err := EnsureKey()
	if err != nil || !created || !strings.HasPrefix(pub, "age1") {
		t.Fatalf("EnsureKey: pub=%q created=%v err=%v", pub, created, err)
	}
	fi, err := os.Stat(paths.AgeKeyFile())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("key perm = %o, want 0600", fi.Mode().Perm())
	}
	b, _ := os.ReadFile(paths.AgeKeyFile())
	if !strings.Contains(string(b), "AGE-SECRET-KEY-1") {
		t.Fatal("key file lacks AGE-SECRET-KEY line")
	}
	pub2, created2, err := EnsureKey()
	if err != nil || created2 || pub2 != pub {
		t.Fatalf("second EnsureKey: pub=%q created=%v err=%v", pub2, created2, err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	repo := sandbox(t)
	if err := Save(repo, map[string]string{"A": "1"}); err == nil {
		t.Fatal("Save without key should fail")
	}
	if _, _, err := EnsureKey(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"TOKEN": "hunter2-plaintext", "A": "x=y"}
	if err := Save(repo, want); err != nil {
		t.Fatal(err)
	}
	ct, err := os.ReadFile(filepath.Join(repo, File))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(ct), "-----BEGIN AGE ENCRYPTED FILE-----") {
		t.Fatal("ciphertext is not armored")
	}
	if strings.Contains(string(ct), "hunter2") || strings.Contains(string(ct), "TOKEN") {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["TOKEN"] != "hunter2-plaintext" || got["A"] != "x=y" {
		t.Fatalf("Load = %v, want %v", got, want)
	}
}

func TestApplyMergesAndReplaces(t *testing.T) {
	repo := sandbox(t)
	if _, _, err := EnsureKey(); err != nil {
		t.Fatal(err)
	}
	env := paths.ClaudeEnvFile()
	if err := os.MkdirAll(filepath.Dir(env), 0o755); err != nil {
		t.Fatal(err)
	}
	const above = "USER_A=1\n  # keep me \n"
	const below = "USER_B=2\nno trailing newline"
	if err := os.WriteFile(env, []byte(above+beginMarker+"\nOLD=stale\n"+endMarker+"\n"+below), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(repo, map[string]string{"Z": "last", "A": "first"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(repo); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(env)
	want := above + beginMarker + "\nA=first\nZ=last\n" + endMarker + "\n" + below
	if string(got) != want {
		t.Fatalf("after first Apply:\n%q\nwant:\n%q", got, want)
	}
	fi, _ := os.Stat(env)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf(".env perm = %o, want 0600", fi.Mode().Perm())
	}

	if err := Save(repo, map[string]string{"A": "changed"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(repo); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(env)
	want = above + beginMarker + "\nA=changed\n" + endMarker + "\n" + below
	if string(got) != want {
		t.Fatalf("after second Apply:\n%q\nwant:\n%q", got, want)
	}
}

func TestApplyAppendsBlock(t *testing.T) {
	repo := sandbox(t)
	if _, _, err := EnsureKey(); err != nil {
		t.Fatal(err)
	}
	env := paths.ClaudeEnvFile()
	if err := os.MkdirAll(filepath.Dir(env), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("X=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(repo, map[string]string{"K": "v"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(repo); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(env)
	want := "X=1\n\n" + beginMarker + "\nK=v\n" + endMarker + "\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyNoSecretsFile(t *testing.T) {
	repo := sandbox(t)
	if err := Apply(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.ClaudeEnvFile()); !os.IsNotExist(err) {
		t.Fatalf(".env should not exist, stat err = %v", err)
	}
}

func TestApplyNoKeyWarnsAndLeavesEnv(t *testing.T) {
	repo := sandbox(t)
	if err := os.WriteFile(filepath.Join(repo, File), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := paths.ClaudeEnvFile()
	if err := os.MkdirAll(filepath.Dir(env), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("X=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Apply(repo); err != nil {
		t.Fatalf("Apply without key should be nil, got %v", err)
	}
	got, _ := os.ReadFile(env)
	if string(got) != "X=1\n" {
		t.Fatalf(".env modified: %q", got)
	}
}
