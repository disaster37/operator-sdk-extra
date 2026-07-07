package test

import (
	"errors"
	"time"
)

var ErrEventuallyTimeout = errors.New("timed out waiting for condition")

func RunWithTimeout(f func() error, timeout time.Duration, interval time.Duration) (isTimeout bool, err error) {
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutTimer.C:
			return true, err
		case <-ticker.C:
			if err = f(); err == nil {
				return false, nil
			}
		}
	}
}
