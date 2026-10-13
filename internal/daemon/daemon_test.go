package daemon

import (
	"runtime"
	"strings"
	"testing"
)

func has(t *testing.T, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			t.Errorf("missing %q in:\n%s", sub, s)
		}
	}
}

func TestPlist(t *testing.T) {
	has(t, PlistTemplate("/x/pine", 15, "/h/daemon.log"),
		"<string>/x/pine</string>", "<integer>900</integer>", "<string>/h/daemon.log</string>", "com.pineapple.pine", "/opt/homebrew/bin")
}

func TestSystemd(t *testing.T) {
	has(t, TimerTemplate(15), "OnUnitActiveSec=15min", "OnBootSec=2min", "Persistent=true")
	has(t, ServiceTemplate("/x/pine"), "ExecStart=/x/pine sync --quiet", "Type=oneshot")
}

func TestCronLine(t *testing.T) {
	has(t, CronLine("/x/pine", 15, "/h/d.log"), "*/15 * * * * /x/pine sync --quiet >> /h/d.log 2>&1", cronMarker)
	has(t, CronLine("/x/pine", 120, "/h/d.log"), "*/59 ")
}

func TestDetectDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" && Detect() != Launchd {
		t.Error("want launchd on darwin")
	}
}
