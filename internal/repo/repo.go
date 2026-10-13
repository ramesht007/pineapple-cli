// Package repo wraps the git and gh CLIs.
package repo

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// AddedLine matches scan.AddedLine (identical type).
type AddedLine = struct{ File, Line string }

// ErrNonFastForward means the push was rejected because the remote moved.
var ErrNonFastForward = errors.New("push rejected: remote has newer commits")

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

func Clone(url, dir string) error {
	_, err := git("", "clone", "--quiet", url, dir)
	return err
}

func Fetch(dir string) error {
	_, err := git(dir, "fetch", "--quiet", "origin")
	return err
}

// OriginRef returns the remote's default-branch ref, or "" if the remote is empty.
func OriginRef(dir string) string {
	for _, r := range []string{"origin/HEAD", "origin/main", "origin/master"} {
		if _, err := git(dir, "rev-parse", "--verify", "--quiet", r); err == nil {
			return r
		}
	}
	return ""
}

// OriginSHA returns the commit sha of the remote's default branch, or "" if the remote is empty.
func OriginSHA(dir string) (string, error) {
	ref := OriginRef(dir)
	if ref == "" {
		return "", nil
	}
	out, err := git(dir, "rev-parse", ref)
	return strings.TrimSpace(out), err
}

// HeadSHA returns the commit sha of the clone's HEAD.
func HeadSHA(dir string) (string, error) {
	out, err := git(dir, "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

// ResetToOriginHead makes the clone match the remote exactly (it is only a cache).
func ResetToOriginHead(dir string) error {
	if err := Fetch(dir); err != nil {
		return err
	}
	ref := OriginRef(dir)
	if ref == "" {
		return errors.New("remote has no commits yet; run `pine push` first")
	}
	if _, err := git(dir, "reset", "--hard", "--quiet", ref); err != nil {
		return err
	}
	_, err := git(dir, "clean", "-fdq")
	return err
}

func CheckoutMain(dir string) error {
	_, err := git(dir, "checkout", "--quiet", "-B", "main")
	return err
}

// Dirty reports whether the working tree differs from HEAD.
func Dirty(dir string) (bool, error) {
	out, err := git(dir, "status", "--porcelain")
	return strings.TrimSpace(out) != "", err
}

// Reset unstages everything.
func Reset(dir string) error {
	_, err := git(dir, "reset", "--quiet")
	return err
}

// AddAllCommitPush stages, commits and pushes; committed=false if nothing changed.
func AddAllCommitPush(dir, msg string) (committed bool, err error) {
	if _, err = git(dir, "add", "-A"); err != nil {
		return false, err
	}
	dirty, err := Dirty(dir)
	if err != nil || !dirty {
		return false, err
	}
	if _, err = git(dir, "commit", "--quiet", "-m", msg); err != nil {
		return false, err
	}
	if _, err = git(dir, "push", "--quiet", "-u", "origin", "HEAD"); err != nil {
		s := err.Error()
		if strings.Contains(s, "rejected") || strings.Contains(s, "non-fast-forward") || strings.Contains(s, "fetch first") {
			return true, fmt.Errorf("%w (%v)", ErrNonFastForward, err)
		}
		return true, err
	}
	return true, nil
}

// DiffCachedAddedLines stages everything and returns the added lines per file.
func DiffCachedAddedLines(dir string) ([]AddedLine, error) {
	if _, err := git(dir, "add", "-A"); err != nil {
		return nil, err
	}
	out, err := git(dir, "-c", "core.quotepath=off", "diff", "--cached", "--no-color", "-U0")
	if err != nil {
		return nil, err
	}
	var lines []AddedLine
	file := ""
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(l, "+++ "), "b/")
			if file == "/dev/null" {
				file = ""
			}
		case strings.HasPrefix(l, "+") && file != "":
			lines = append(lines, AddedLine{file, l[1:]})
		}
	}
	return lines, nil
}

// RemoteAhead fetches and reports whether origin has commits HEAD lacks.
func RemoteAhead(dir string) (bool, error) {
	if err := Fetch(dir); err != nil {
		return false, err
	}
	ref := OriginRef(dir)
	if ref == "" {
		return false, nil
	}
	out, err := git(dir, "rev-list", "HEAD.."+ref, "--count")
	if err != nil { // unborn local HEAD etc.
		return false, nil
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n > 0, nil
}

// EnsureGH checks gh is installed and authenticated; it never installs anything.
func EnsureGH() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return errors.New("GitHub CLI `gh` not found. Install it (macOS: `brew install gh`; Debian/Ubuntu: `sudo apt install gh`), then run `gh auth login`")
	}
	if err := exec.Command("gh", "auth", "status").Run(); err != nil {
		return errors.New("`gh` is not authenticated. Run `gh auth login` and retry")
	}
	if out, err := exec.Command("gh", "auth", "setup-git").CombinedOutput(); err != nil {
		return fmt.Errorf("gh auth setup-git: %w: %s (run `gh auth login` with HTTPS and retry)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CreateRepo creates a GitHub repo under the authenticated user and returns its HTTPS clone URL.
func CreateRepo(name string, private bool) (string, error) {
	vis := "--public"
	if private {
		vis = "--private"
	}
	if out, err := exec.Command("gh", "repo", "create", name, vis).CombinedOutput(); err != nil {
		return "", fmt.Errorf("gh repo create: %w: %s", err, strings.TrimSpace(string(out)))
	}
	out, err := exec.Command("gh", "api", "user", "--jq", ".login").Output()
	if err != nil {
		return "", fmt.Errorf("gh api user: %w", err)
	}
	return fmt.Sprintf("https://github.com/%s/%s.git", strings.TrimSpace(string(out)), name), nil
}
