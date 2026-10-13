package lockfile

import (
	"path/filepath"
	"testing"
)

func TestAcquireTwice(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock")
	rel, err := acquire(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquire(p); err == nil {
		t.Fatal("second acquire should fail while held")
	}
	rel()
	rel2, err := acquire(p)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	rel2()
}
