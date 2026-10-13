// Package daemon registers an OS-level schedule that runs `pine sync --quiet`.
package daemon

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Backend string

const (
	Launchd Backend = "launchd"
	Systemd Backend = "systemd"
	Cron    Backend = "cron"

	label      = "com.pineapple.pine"
	cronMarker = "# pineapple-cli"
)

// Detect picks the scheduler for this OS.
func Detect() Backend {
	if runtime.GOOS == "darwin" {
		return Launchd
	}
	if exec.Command("systemctl", "--user", "show-environment").Run() == nil {
		return Systemd
	}
	return Cron
}

// Exe returns the absolute, symlink-resolved path of the running binary.
func Exe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe, nil
}

// ---- pure templates ----

func PlistTemplate(exe string, minutes int, logPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>sync</string>
    <string>--quiet</string>
  </array>
  <key>StartInterval</key><integer>%d</integer>
  <key>RunAtLoad</key><true/>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin</string>
  </dict>
</dict>
</plist>
`, label, exe, minutes*60, logPath, logPath)
}

func ServiceTemplate(exe string) string {
	return fmt.Sprintf("[Unit]\nDescription=Pineapple CLI sync\n\n[Service]\nType=oneshot\nExecStart=%s sync --quiet\n", exe)
}

func TimerTemplate(minutes int) string {
	return fmt.Sprintf("[Unit]\nDescription=Run pine sync periodically\n\n[Timer]\nOnBootSec=2min\nOnUnitActiveSec=%dmin\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n", minutes)
}

func CronLine(exe string, minutes int, logPath string) string {
	if minutes > 59 {
		minutes = 59 // ponytail: */N only fits within the hour; hourly+ would need an hours field
	}
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("*/%d * * * * %s sync --quiet >> %s 2>&1 %s", minutes, exe, logPath, cronMarker)
}

// ---- paths ----

func plistPath() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library", "LaunchAgents", label+".plist")
}

func systemdDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "systemd", "user")
}

// ---- operations ----

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func write(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// Install registers the schedule for backend b.
func Install(b Backend, exe string, minutes int, logPath string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	switch b {
	case Launchd:
		p := plistPath()
		if err := write(p, PlistTemplate(exe, minutes, logPath)); err != nil {
			return fmt.Errorf("write plist: %w", err)
		}
		_ = exec.Command("launchctl", "unload", p).Run()
		return run("launchctl", "load", p)
	case Systemd:
		d := systemdDir()
		if err := write(filepath.Join(d, "pine-sync.service"), ServiceTemplate(exe)); err != nil {
			return fmt.Errorf("write service: %w", err)
		}
		if err := write(filepath.Join(d, "pine-sync.timer"), TimerTemplate(minutes)); err != nil {
			return fmt.Errorf("write timer: %w", err)
		}
		if err := run("systemctl", "--user", "daemon-reload"); err != nil {
			return err
		}
		return run("systemctl", "--user", "enable", "--now", "pine-sync.timer")
	default:
		lines, err := crontab()
		if err != nil {
			return err
		}
		return setCrontab(append(lines, CronLine(exe, minutes, logPath)))
	}
}

func Uninstall(b Backend) error {
	switch b {
	case Launchd:
		p := plistPath()
		_ = exec.Command("launchctl", "unload", p).Run()
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove plist: %w", err)
		}
		return nil
	case Systemd:
		_ = exec.Command("systemctl", "--user", "disable", "--now", "pine-sync.timer").Run()
		for _, f := range []string{"pine-sync.timer", "pine-sync.service"} {
			if err := os.Remove(filepath.Join(systemdDir(), f)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove %s: %w", f, err)
			}
		}
		return run("systemctl", "--user", "daemon-reload")
	default:
		lines, err := crontab()
		if err != nil {
			return err
		}
		return setCrontab(lines)
	}
}

// Status reports whether the schedule is active.
func Status(b Backend) bool {
	switch b {
	case Launchd:
		return exec.Command("launchctl", "list", label).Run() == nil
	case Systemd:
		return exec.Command("systemctl", "--user", "is-active", "pine-sync.timer").Run() == nil
	default:
		out, err := exec.Command("crontab", "-l").Output()
		return err == nil && strings.Contains(string(out), cronMarker)
	}
}

// crontab returns current crontab lines without ours (empty if none exists).
func crontab() ([]string, error) {
	if _, err := exec.LookPath("crontab"); err != nil {
		return nil, errors.New("`crontab` not found; install cron or use `pine daemon run`")
	}
	out, _ := exec.Command("crontab", "-l").Output() // non-zero when no crontab yet
	var keep []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if l != "" && !strings.Contains(l, cronMarker) {
			keep = append(keep, l)
		}
	}
	return keep, nil
}

func setCrontab(lines []string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = bytes.NewBufferString(strings.Join(lines, "\n") + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("crontab -: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
