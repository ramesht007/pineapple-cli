// Package syncer mirrors ~/.claude skills/mods to and from the sync repo clone.
package syncer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ramesht007/pineapple-cli/internal/paths"
)

type root struct{ prefix, repo, local string }

func roots(repoDir, skillsDir, modsDir string) []root {
	return []root{
		{"skills", filepath.Join(repoDir, "skills"), skillsDir},
		{"mods", filepath.Join(repoDir, "mods"), modsDir},
	}
}

// walk returns relpath ("<prefix>/a/b", slash-separated) -> abs path of every
// syncable regular file under dir. Skips skills/synced, .DS_Store, .git, symlinks.
func walk(dir, prefix string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if p != dir && (d.Name() == ".git" || (prefix == "skills" && p == filepath.Join(dir, "synced"))) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == ".DS_Store" || !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		out[prefix+"/"+filepath.ToSlash(rel)] = p
		return nil
	})
	return out, err
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashIfExists returns ("", false, nil) for a missing file.
func hashIfExists(p string) (string, bool, error) {
	h, err := hashFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	return h, err == nil, err
}

func hashTree(skillsDir, modsDir string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range [][2]string{{"skills", skillsDir}, {"mods", modsDir}} {
		files, err := walk(r[1], r[0])
		if err != nil {
			return nil, err
		}
		for rel, p := range files {
			if out[rel], err = hashFile(p); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// Rehash hashes everything the repo clone holds under skills/ and mods/.
func Rehash(repoDir string) (map[string]string, error) {
	return hashTree(filepath.Join(repoDir, "skills"), filepath.Join(repoDir, "mods"))
}

// LocalHashes hashes the syncable files in the Claude dirs.
func LocalHashes(skillsDir, modsDir string) (map[string]string, error) {
	return hashTree(skillsDir, modsDir)
}

// copyFile copies atomically (temp + rename), keeping the permission bits.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".pine-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(fi.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// removeFile deletes p and prunes now-empty parent dirs up to (not including) top.
func removeFile(top, p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for d := filepath.Dir(p); d != top && strings.HasPrefix(d, top); d = filepath.Dir(d) {
		if os.Remove(d) != nil { // non-empty
			break
		}
	}
	return nil
}

func dest(dir, prefix, rel string) string {
	return filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(rel, prefix+"/")))
}

// Snapshot mirrors local skills/mods into the repo clone: copy new/changed
// files, delete repo files that no longer exist locally.
func Snapshot(skillsDir, modsDir, repoDir string) error {
	for _, r := range roots(repoDir, skillsDir, modsDir) {
		src, err := walk(r.local, r.prefix)
		if err != nil {
			return fmt.Errorf("scan %s: %w", r.local, err)
		}
		dst, err := walk(r.repo, r.prefix)
		if err != nil {
			return fmt.Errorf("scan %s: %w", r.repo, err)
		}
		for rel, sp := range src {
			dp := dest(r.repo, r.prefix, rel)
			if dpath, ok := dst[rel]; ok {
				sh, err1 := hashFile(sp)
				dh, err2 := hashFile(dpath)
				if err1 == nil && err2 == nil && sh == dh {
					continue
				}
			}
			if err := copyFile(sp, dp); err != nil {
				return fmt.Errorf("copy %s: %w", rel, err)
			}
		}
		for rel, dp := range dst {
			if _, ok := src[rel]; !ok {
				if err := removeFile(r.repo, dp); err != nil {
					return fmt.Errorf("remove %s: %w", rel, err)
				}
			}
		}
	}
	return nil
}

// Apply brings repo contents into the Claude dirs. Remote wins on conflict, but a
// locally modified file is first copied to BackupsDir()/<timestamp>/<relpath>.
// conflicts entries look like "<relpath> -> <backup path>".
// A file is only overwritten if the remote changed it since the last sync (hash != state);
// local edits against an unchanged remote are left for the next push.
func Apply(repoDir, skillsDir, modsDir string, state map[string]string) (applied, conflicts []string, err error) {
	stamp := time.Now().Format("20060102-150405")
	backup := func(rel, localPath string) error {
		bp := filepath.Join(paths.BackupsDir(), stamp, filepath.FromSlash(rel))
		if err := copyFile(localPath, bp); err != nil {
			return fmt.Errorf("backup %s: %w", rel, err)
		}
		conflicts = append(conflicts, rel+" -> "+bp)
		return nil
	}
	inRepo := map[string]bool{}
	rs := roots(repoDir, skillsDir, modsDir)
	for _, r := range rs {
		files, err := walk(r.repo, r.prefix)
		if err != nil {
			return applied, conflicts, err
		}
		rels := make([]string, 0, len(files))
		for rel := range files {
			rels = append(rels, rel)
			inRepo[rel] = true
		}
		sort.Strings(rels)
		for _, rel := range rels {
			rh, err := hashFile(files[rel])
			if err != nil {
				return applied, conflicts, err
			}
			lp := dest(r.local, r.prefix, rel)
			lh, exists, err := hashIfExists(lp)
			if err != nil {
				return applied, conflicts, err
			}
			if (exists && lh == rh) || rh == state[rel] {
				continue
			}
			if exists && lh != state[rel] {
				if err := backup(rel, lp); err != nil {
					return applied, conflicts, err
				}
			}
			if err := copyFile(files[rel], lp); err != nil {
				return applied, conflicts, fmt.Errorf("apply %s: %w", rel, err)
			}
			applied = append(applied, rel)
		}
	}
	gone := make([]string, 0)
	for rel := range state {
		if !inRepo[rel] {
			gone = append(gone, rel)
		}
	}
	sort.Strings(gone)
	for _, rel := range gone {
		for _, r := range rs {
			if !strings.HasPrefix(rel, r.prefix+"/") {
				continue
			}
			lp := dest(r.local, r.prefix, rel)
			lh, exists, err := hashIfExists(lp)
			if err != nil {
				return applied, conflicts, err
			}
			if !exists {
				break
			}
			if lh != state[rel] {
				if err := backup(rel, lp); err != nil {
					return applied, conflicts, err
				}
			}
			if err := removeFile(r.local, lp); err != nil {
				return applied, conflicts, err
			}
			applied = append(applied, rel)
		}
	}
	return applied, conflicts, nil
}
