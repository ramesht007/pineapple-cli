package scan

import "testing"

func TestScan(t *testing.T) {
	tests := []struct {
		name, file, line, rule string // rule "" = expect no findings
	}{
		{"aws", "skills/a/x.sh", "export K=AKIAABCDEFGHIJKLMNOP", "aws-access-key"},
		{"ghp", "skills/a/x.md", "ghp_" + "abcdefghijklmnopqrstuvwxyzABCDEFGHIJ", "github-token"},
		{"pat", "mods/m/c.json", "github_pat_11ABCDEFG0123456789_xyz", "github-pat"},
		{"anthropic", "skills/a/x", "key sk-ant-api03-abcdefghijklmnopqrst", "anthropic-key"},
		{"pem", "skills/a/id", "-----BEGIN OPENSSH PRIVATE KEY-----", "private-key"},
		{"pem-plain", "skills/a/id", "-----BEGIN PRIVATE KEY-----", "private-key"},
		{"generic", "mods/m/c.json", `"api_key": "abcdEFGH12345678xyz"`, "generic-secret"},
		{"generic-pass", "mods/m/c.env", "PASSWORD=hunter2hunter2hunter2", "generic-secret"},
		{"clean prose", "skills/a/SKILL.md", "Use the token to authenticate", ""},
		{"short value", "mods/m/c.json", `"password": "short"`, ""},
		{"normal code", "skills/a/x.go", "func main() { fmt.Println(1) }", ""},
		{"age file exempt", "secrets.env.age", "AKIAABCDEFGHIJKLMNOP password=abcdefghijklmnop1234", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Scan([]AddedLine{{tt.file, tt.line}})
			if tt.rule == "" {
				if len(got) != 0 {
					t.Fatalf("want no findings, got %v", got)
				}
				return
			}
			for _, f := range got {
				if f.Rule == tt.rule && f.File == tt.file {
					return
				}
			}
			t.Fatalf("want rule %s, got %v", tt.rule, got)
		})
	}
}
