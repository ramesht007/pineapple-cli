// Package ui holds pine's palette, styled-string helpers, banner, spinner and boxes.
// Everything degrades to plain text when stdout is not a TTY or NO_COLOR is set.
package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	gold  = lipgloss.Color("220") // #FFD700
	green = lipgloss.Color("112") // #9ACD32
	red   = lipgloss.Color("196")
	dim   = lipgloss.Color("245")
)

// Quiet suppresses the banner and spinner output (set from --quiet).
var Quiet bool

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

var (
	stdoutTTY = isTTY(os.Stdout)
	noColor   = os.Getenv("NO_COLOR") != ""
	// colorOn is computed once at init; tests flip it via SetColor.
	colorOn = stdoutTTY && !noColor
)

// SetColor overrides color output (for tests).
func SetColor(on bool) { colorOn = on }

func paint(c lipgloss.Color, s string, bold bool) string {
	if !colorOn {
		return s
	}
	return lipgloss.NewStyle().Foreground(c).Bold(bold).Render(s)
}

func Good(s string) string { return paint(green, s, false) }
func Bad(s string) string  { return paint(red, s, false) }
func Warn(s string) string { return paint(gold, s, true) }
func Dim(s string) string  { return paint(dim, s, false) }
func Gold(s string) string { return paint(gold, s, false) }

// RelTime renders t as "2h ago", "3d ago", or "never" for the zero time.
func RelTime(t time.Time) string { return relTime(t, time.Now()) }

func relTime(t, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

const figPineapple = `██████╗ ██╗███╗   ██╗███████╗ █████╗ ██████╗ ██████╗ ██╗     ███████╗
██╔══██╗██║████╗  ██║██╔════╝██╔══██╗██╔══██╗██╔══██╗██║     ██╔════╝
██████╔╝██║██╔██╗ ██║█████╗  ███████║██████╔╝██████╔╝██║     █████╗
██╔═══╝ ██║██║╚██╗██║██╔══╝  ██╔══██║██╔═══╝ ██╔═══╝ ██║     ██╔══╝
██║     ██║██║ ╚████║███████╗██║  ██║██║     ██║     ███████╗███████╗
╚═╝     ╚═╝╚═╝  ╚═══╝╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝     ╚══════╝╚══════╝`

const figCLI = ` ██████╗██╗     ██╗
██╔════╝██║     ██║
██║     ██║     ██║
╚██████╗███████╗██║
 ╚═════╝╚══════╝╚═╝`

var crown = []string{
	`  ▲  ▲  ▲  `,
	` ▲ ▲ ▲ ▲ ▲ `,
	`  ▲ ▲▲▲ ▲  `,
	`   ▲▲▲▲▲   `,
	`    ▲▲▲    `,
}

var body = []string{
	`  ▒█▓█▓█▒  `,
	` ▒█▓█▓█▓█▒ `,
	`▒█▓█▓█▓█▓█▒`,
	`▒▓█▓█▓█▓█▓▒`,
	`▒█▓█▓█▓█▓█▒`,
	`▒▓█▓█▓█▓█▓▒`,
	` ▒█▓█▓█▓█▒ `,
	`  ▒▓█▓█▓▒  `,
	`   ▀▀▀▀▀   `,
}

// Banner renders the startup banner for the given version (ends with a newline).
func Banner(version string) string {
	var p []string
	for _, l := range crown {
		p = append(p, Good(l))
	}
	for _, l := range body {
		p = append(p, Gold(l))
	}
	art := lipgloss.JoinVertical(lipgloss.Left, p...)
	// pineapple on the left, PINEAPPLE beside it, CLI directly below the P
	right := lipgloss.JoinVertical(lipgloss.Left,
		Gold(figPineapple), "",
		Gold(figCLI), "",
		Dim("v"+version+" · by Ramesh Shukla (@ramesht007)"))
	// joined as strings; colour is applied per-line so JoinHorizontal measures width correctly
	return lipgloss.JoinHorizontal(lipgloss.Center, art, " ", right) + "\n"
}

// IsStdoutTTY reports whether stdout is a terminal.
func IsStdoutTTY() bool { return stdoutTTY }

// ShowBanner prints the banner to stdout when stdout is a TTY and not --quiet.
func ShowBanner(version string) {
	if stdoutTTY && !Quiet {
		fmt.Print(Banner(version), "\n")
	}
}

// Box renders lines inside a gold rounded border with a title.
func Box(title string, lines ...string) string {
	content := strings.Join(lines, "\n")
	if !colorOn {
		return "[" + title + "]\n" + content + "\n"
	}
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(gold).Padding(0, 1)
	return Gold(" "+title+" ") + "\n" + s.Render(content) + "\n"
}

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spin shows an animated spinner on stderr; call the returned func to finish.
// Without a TTY (or with NO_COLOR/--quiet) it prints label once and never animates.
func Spin(label string) (stop func(ok bool)) {
	if Quiet {
		return func(bool) {}
	}
	if !isTTY(os.Stderr) || noColor {
		fmt.Fprintln(os.Stderr, label)
		return func(bool) {}
	}
	done, fin := make(chan struct{}), sync.WaitGroup{}
	fin.Add(1)
	go func() {
		defer fin.Done()
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for i := 0; ; i++ {
			fmt.Fprintf(os.Stderr, "\r\033[K%s %s", lipgloss.NewStyle().Foreground(gold).Render(frames[i%len(frames)]), label)
			select {
			case <-done:
				return
			case <-t.C:
			}
		}
	}()
	return func(ok bool) {
		close(done)
		fin.Wait()
		mark := lipgloss.NewStyle().Foreground(green).Render("✓")
		if !ok {
			mark = lipgloss.NewStyle().Foreground(red).Render("✗")
		}
		fmt.Fprintf(os.Stderr, "\r\033[K%s %s\n", mark, label)
	}
}
