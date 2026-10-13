// Package bootstrap makes sure git and an authenticated gh are available, installing them on request.
package bootstrap

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const manualURL = "https://cli.github.com"

// Ensure checks git and gh, offers to install what is missing, and walks through gh auth.
func Ensure(assumeYes bool) error {
	if err := ensureGit(assumeYes); err != nil {
		return err
	}
	if err := ensureGHInstalled(assumeYes); err != nil {
		return err
	}
	if exec.Command("gh", "auth", "status").Run() != nil {
		fmt.Fprintln(os.Stderr, "GitHub login needed — opening device/web flow")
		if err := runInteractive("gh", "auth", "login", "--hostname", "github.com", "--git-protocol", "https", "--web"); err != nil {
			return fmt.Errorf("gh auth login: %w (run `gh auth login` manually and retry)", err)
		}
	}
	if out, err := exec.Command("gh", "auth", "setup-git").CombinedOutput(); err != nil {
		return fmt.Errorf("gh auth setup-git: %w: %s (run `gh auth login` with HTTPS and retry)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureGit(assumeYes bool) error {
	if _, err := exec.LookPath("git"); err == nil {
		return nil
	}
	if runtime.GOOS == "darwin" && !onPath("brew") {
		if !confirm("git is missing. Run `xcode-select --install`?", assumeYes) {
			return errors.New("git not found; install it and retry")
		}
		if err := runInteractive("xcode-select", "--install"); err != nil {
			return fmt.Errorf("xcode-select --install: %w", err)
		}
		return errors.New("rerun pine init after the install finishes")
	}
	cmd, err := installCmd("git", runtime.GOOS, onPath, os.Geteuid() == 0)
	if err != nil {
		return fmt.Errorf("git not found: %w", err)
	}
	return runInstall("git", cmd, assumeYes, "https://git-scm.com/downloads")
}

func ensureGHInstalled(assumeYes bool) error {
	if onPath("gh") {
		return nil
	}
	cmd, err := installCmd("gh", runtime.GOOS, onPath, os.Geteuid() == 0)
	if err != nil {
		return fmt.Errorf("GitHub CLI `gh` not found: %w; install it from %s", err, manualURL)
	}
	if err := runInstall("gh", cmd, assumeYes, manualURL); err != nil {
		return err
	}
	if !onPath("gh") {
		return fmt.Errorf("gh installed but not on PATH; open a new shell and rerun pine init (or see %s)", manualURL)
	}
	return nil
}

func runInstall(tool, cmd string, assumeYes bool, manual string) error {
	if !confirm(fmt.Sprintf("%s is missing. Install with `%s`?", tool, cmd), assumeYes) {
		return fmt.Errorf("%s not installed; install it from %s and retry", tool, manual)
	}
	if err := runInteractive("sh", "-c", cmd); err != nil {
		return fmt.Errorf("installing %s failed: %w; install it manually from %s", tool, err, manual)
	}
	return nil
}

// installCmd returns the shell command that installs tool ("git" or "gh") on goos.
func installCmd(tool, goos string, has func(string) bool, root bool) (string, error) {
	pkg := tool
	var cmd string
	switch {
	case goos == "darwin":
		if !has("brew") {
			return "", errors.New("Homebrew (brew) not found")
		}
		return "brew install " + pkg, nil
	case goos != "linux":
		return "", fmt.Errorf("unsupported OS %q", goos)
	case has("apt-get"):
		cmd = "sudo apt-get update && sudo apt-get install -y " + pkg
	case has("dnf"):
		cmd = "sudo dnf install -y " + pkg
	case has("yum"):
		cmd = "sudo yum install -y " + pkg
	case has("apk"):
		if tool == "gh" {
			pkg = "github-cli"
		}
		cmd = "sudo apk add " + pkg
	case has("pacman"):
		if tool == "gh" {
			pkg = "github-cli"
		}
		cmd = "sudo pacman -S --noconfirm " + pkg
	default:
		return "", errors.New("no supported package manager found (apt-get, dnf, yum, apk, pacman)")
	}
	if root {
		cmd = strings.ReplaceAll(cmd, "sudo ", "")
	}
	return cmd, nil
}

func onPath(name string) bool { _, err := exec.LookPath(name); return err == nil }

// confirm asks a y/N question; EOF or anything but y/yes means no, unless assumeYes.
func confirm(q string, assumeYes bool) bool {
	if assumeYes {
		return true
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", q)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes"
}

func runInteractive(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}
