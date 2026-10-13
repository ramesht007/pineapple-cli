package config

import "testing"

func TestStateAppliedSHARoundTrip(t *testing.T) {
	t.Setenv("PINEAPPLE_HOME", t.TempDir())
	if err := SaveState(State{Files: map[string]string{"a": "b"}, AppliedSHA: "abc123"}); err != nil {
		t.Fatal(err)
	}
	s, err := LoadState()
	if err != nil || s.AppliedSHA != "abc123" || s.Files["a"] != "b" {
		t.Fatalf("got %+v, %v", s, err)
	}
}
