package backend

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

const DefaultTransferIdleTimeout = 2 * time.Minute

var ErrTransferIdleTimeout = errors.New("HTTP transfer stalled: idle timeout exceeded")

// A watchdog belongs to one request, including its response body. Cancelling
// that request also interrupts a blocked transport write or response read.
// Progress extends the idle deadline without imposing a total transfer limit.
type transferWatchdog struct {
	ctx          context.Context
	cancel       context.CancelCauseFunc
	mu           sync.Mutex
	timer        *time.Timer
	lastProgress time.Time
	timeout      time.Duration
	stopped      bool
}

func newTransferWatchdog(parent context.Context, timeout time.Duration) *transferWatchdog {
	if timeout <= 0 {
		timeout = DefaultTransferIdleTimeout
	}
	ctx, cancel := context.WithCancelCause(parent)
	w := &transferWatchdog{ctx: ctx, cancel: cancel, timeout: timeout, lastProgress: time.Now()}
	w.mu.Lock()
	w.timer = time.AfterFunc(timeout, w.check)
	w.mu.Unlock()
	return w
}

func (w *transferWatchdog) check() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	if remaining := w.timeout - time.Since(w.lastProgress); remaining > 0 {
		w.timer.Reset(remaining)
		return
	}
	w.stopped = true
	w.cancel(ErrTransferIdleTimeout)
}

func (w *transferWatchdog) progress() {
	w.mu.Lock()
	w.lastProgress = time.Now()
	w.mu.Unlock()
}

func (w *transferWatchdog) stop() {
	w.mu.Lock()
	w.stopped = true
	w.timer.Stop()
	w.cancel(context.Canceled)
	w.mu.Unlock()
}

func (w *transferWatchdog) transferError(err error) error {
	if errors.Is(context.Cause(w.ctx), ErrTransferIdleTimeout) {
		return ErrTransferIdleTimeout
	}
	return err
}

type progressReader struct {
	io.ReadCloser
	watchdog *transferWatchdog
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.watchdog.progress()
	}
	return n, err
}

type watchedResponseBody struct {
	progressReader
}

func (r *watchedResponseBody) Read(p []byte) (int, error) {
	n, err := r.progressReader.Read(p)
	if err != nil {
		err = r.watchdog.transferError(err)
		r.watchdog.stop()
	}
	return n, err
}

func (r *watchedResponseBody) Close() error {
	r.watchdog.stop()
	return r.ReadCloser.Close()
}
