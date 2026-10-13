package bootstrap

import "testing"

func TestInstallCmd(t *testing.T) {
	only := func(names ...string) func(string) bool {
		return func(n string) bool {
			for _, x := range names {
				if x == n {
					return true
				}
			}
			return false
		}
	}
	tests := []struct {
		tool, goos string
		has        func(string) bool
		root       bool
		want       string
		wantErr    bool
	}{
		{"gh", "darwin", only("brew"), false, "brew install gh", false},
		{"gh", "darwin", only(), false, "", true},
		{"gh", "linux", only("apt-get", "dnf"), false, "sudo apt-get update && sudo apt-get install -y gh", false},
		{"gh", "linux", only("apt-get"), true, "apt-get update && apt-get install -y gh", false},
		{"gh", "linux", only("dnf", "yum"), false, "sudo dnf install -y gh", false},
		{"gh", "linux", only("yum"), false, "sudo yum install -y gh", false},
		{"gh", "linux", only("apk"), false, "sudo apk add github-cli", false},
		{"gh", "linux", only("pacman"), false, "sudo pacman -S --noconfirm github-cli", false},
		{"git", "linux", only("apk"), true, "apk add git", false},
		{"gh", "linux", only(), false, "", true},
		{"gh", "windows", only("brew"), false, "", true},
	}
	for _, tc := range tests {
		got, err := installCmd(tc.tool, tc.goos, tc.has, tc.root)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s/%s: got %q, %v; want %q (err=%v)", tc.tool, tc.goos, got, err, tc.want, tc.wantErr)
		}
	}
}
