// Package lockfile serialises pine processes (daemon tick, manual sync, cron) with an flock.
package lockfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/ramesht007/pineapple-cli/internal/paths"
)

// Acquire takes an exclusive, non-blocking lock on PINEAPPLE_HOME/lock.
// The kernel drops it if the process dies.
func Acquire() (release func(), err error) {
	return acquire(filepath.Join(paths.PineappleHome(), "lock"))
}

func acquire(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("another pine process is running; wait for it to finish and retry")
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return func() { f.Close() }, nil // closing the fd releases the flock
}
