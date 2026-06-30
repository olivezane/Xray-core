package signal

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type ActivityUpdater interface {
	Update()
}

type ActivityTimer struct {
	mu        sync.RWMutex
	timer     *time.Timer
	timeout   time.Duration
	onTimeout func()
	consumed  atomic.Bool
	once      sync.Once
}

func (t *ActivityTimer) Update() {
	if t.consumed.Load() {
		return
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.timer != nil && t.timeout > 0 {
		t.timer.Reset(t.timeout)
	}
}

func (t *ActivityTimer) finish() {
	t.once.Do(func() {
		t.consumed.Store(true)
		t.mu.Lock()
		defer t.mu.Unlock()

		if t.timer != nil {
			t.timer.Stop()
			t.timer = nil
		}
		t.onTimeout()
	})
}

func (t *ActivityTimer) SetTimeout(timeout time.Duration) {
	if t.consumed.Load() {
		return
	}
	if timeout == 0 {
		t.finish()
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	// double check, just in case
	if t.consumed.Load() {
		return
	}
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timeout = timeout
	t.timer = time.AfterFunc(timeout, t.finish)
}

func CancelAfterInactivity(ctx context.Context, cancel context.CancelFunc, timeout time.Duration) *ActivityTimer {
	timer := &ActivityTimer{
		onTimeout: cancel,
	}
	timer.SetTimeout(timeout)
	return timer
}
