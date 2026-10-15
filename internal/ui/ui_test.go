package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{time.Time{}, "never"},
		{now.Add(-10 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-2 * time.Hour), "2h ago"},
		{now.Add(-72 * time.Hour), "3d ago"},
	} {
		if got := relTime(c.t, now); got != c.want {
			t.Errorf("relTime = %q, want %q", got, c.want)
		}
	}
}

func TestBannerWidth(t *testing.T) {
	SetColor(true)
	defer SetColor(false)
	out := ansi.ReplaceAllString(Banner("1.2.3"), "")
	if !strings.Contains(out, "v1.2.3") {
		t.Error("version missing")
	}
	for _, l := range strings.Split(out, "\n") {
		if n := len([]rune(l)); n > 90 {
			t.Errorf("line %d runes > 90: %q", n, l)
		}
	}
}

func TestPlainPassthrough(t *testing.T) {
	SetColor(false)
	for _, f := range []func(string) string{Good, Bad, Warn, Dim, Gold} {
		if f("x") != "x" {
			t.Error("styled output with color off")
		}
	}
}
