// Package secrets syncs one age-encrypted env file through the (public) repo
// and merges it into ~/.claude/.env. The age identity lives at
// PINEAPPLE_HOME/age.key and is copied between machines by hand.
package secrets

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"

	"github.com/ramesht007/pineapple-cli/internal/paths"
)

// File is the ciphertext's name inside the sync repo.
const File = "secrets.env.age"

const (
	beginMarker = "# >>> pineapple managed >>>"
	endMarker   = "# <<< pineapple managed <<<"
)

var errNoKey = errors.New("no age key; run `pine secrets init`")

// loadIdentity parses PINEAPPLE_HOME/age.key (comment lines starting with # are skipped).
func loadIdentity() (*age.X25519Identity, error) {
	b, err := os.ReadFile(paths.AgeKeyFile())
	if err != nil {
		return nil, err
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		id, err := age.ParseX25519Identity(l)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", paths.AgeKeyFile(), err)
		}
		return id, nil
	}
	return nil, fmt.Errorf("parse %s: no AGE-SECRET-KEY line", paths.AgeKeyFile())
}

// EnsureKey loads the age identity, generating one if missing, and returns its public key.
func EnsureKey() (recipient string, created bool, err error) {
	id, err := loadIdentity()
	if err == nil {
		return id.Recipient().String(), false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	if id, err = age.GenerateX25519Identity(); err != nil {
		return "", false, fmt.Errorf("generate age key: %w", err)
	}
	content := fmt.Sprintf("# created by pine %s\n# public key: %s\n%s\n",
		time.Now().UTC().Format(time.RFC3339), id.Recipient(), id)
	if err := writeAtomic(paths.AgeKeyFile(), []byte(content), 0o600); err != nil {
		return "", false, fmt.Errorf("write age key: %w", err)
	}
	return id.Recipient().String(), true, nil
}

// Load decrypts repoDir/secrets.env.age; a missing file yields an empty map.
func Load(repoDir string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(repoDir, File))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	id, err := loadIdentity()
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNoKey
	}
	if err != nil {
		return nil, err
	}
	var r io.Reader = bytes.NewReader(b)
	if bytes.HasPrefix(b, []byte(armor.Header)) {
		r = armor.NewReader(r)
	}
	dr, err := age.Decrypt(r, id)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s: %w", File, err)
	}
	plain, err := io.ReadAll(dr)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s: %w", File, err)
	}
	return Parse(string(plain))
}

// Save encrypts m (armored) to repoDir/secrets.env.age for our own recipient.
func Save(repoDir string, m map[string]string) error {
	id, err := loadIdentity()
	if errors.Is(err, os.ErrNotExist) {
		return errNoKey
	}
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	aw := armor.NewWriter(&buf)
	w, err := age.Encrypt(aw, id.Recipient())
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	if _, err := io.WriteString(w, Render(m)); err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	if err := aw.Close(); err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	return writeAtomic(filepath.Join(repoDir, File), buf.Bytes(), 0o644)
}

// Apply decrypts the repo's secrets and merges them into CLAUDE_DIR/.env.
// No secrets file: nothing to do. No key: warn once and leave .env alone.
func Apply(repoDir string) error {
	if _, err := os.Stat(filepath.Join(repoDir, File)); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if _, err := os.Stat(paths.AgeKeyFile()); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "secrets: %s not applied: no age key at %s (copy it from another machine)\n", File, paths.AgeKeyFile())
		return nil
	}
	m, err := Load(repoDir)
	if err != nil {
		return err
	}
	env := paths.ClaudeEnvFile()
	existing, err := os.ReadFile(env)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeAtomic(env, merge(existing, block(m)), 0o600)
}

// Parse reads KEY=VALUE lines; blank lines and # comments are ignored.
func Parse(s string) (map[string]string, error) {
	m := map[string]string{}
	for i, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", i+1)
		}
		m[strings.TrimSpace(k)] = v
	}
	return m, nil
}

// Render writes m as sorted KEY=VALUE lines.
func Render(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + m[k] + "\n")
	}
	return b.String()
}

func block(m map[string]string) string {
	return beginMarker + "\n" + Render(m) + endMarker + "\n"
}

// merge replaces the marked block in existing, or appends one; everything else is kept verbatim.
func merge(existing []byte, blk string) []byte {
	lines := strings.Split(string(existing), "\n")
	begin, end := -1, -1
	for i, l := range lines {
		if begin < 0 && l == beginMarker {
			begin = i
		} else if begin >= 0 && l == endMarker {
			end = i
			break
		}
	}
	if begin >= 0 && end >= 0 {
		// blk ends with "\n"; drop that so the join restores the original line structure.
		mid := strings.Split(strings.TrimSuffix(blk, "\n"), "\n")
		out := append(append(append([]string{}, lines[:begin]...), mid...), lines[end+1:]...)
		return []byte(strings.Join(out, "\n"))
	}
	out := append([]byte{}, existing...)
	if len(out) > 0 {
		if !bytes.HasSuffix(out, []byte("\n")) {
			out = append(out, '\n')
		}
		out = append(out, '\n')
	}
	return append(out, blk...)
}

// writeAtomic writes data via temp file + rename with the given permissions.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
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
	return os.Rename(tmp.Name(), path)
}
