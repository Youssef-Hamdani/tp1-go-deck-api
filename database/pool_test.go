package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testPool(t *testing.T) *WorkerPool {
	t.Helper()
	p, err := Open(filepath.Join(t.TempDir(), "test.db"), 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}
func TestWALAndConfiguration(t *testing.T) {
	for _, n := range []int{0, 4} {
		if p, err := Open(filepath.Join(t.TempDir(), "bad.db"), n); err == nil {
			p.Close()
			t.Fatal("invalid readers accepted")
		}
	}
	p := testPool(t)
	v, err := p.Do(context.Background(), false, func(ctx context.Context, db *sql.DB) (any, error) {
		var mode string
		err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode)
		return mode, err
	})
	if err != nil || v != "wal" {
		t.Fatalf("WAL: %v %v", v, err)
	}
}
func TestExpiredQueuedJobSkipped(t *testing.T) {
	p := testPool(t)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		p.Do(context.Background(), true, func(context.Context, *sql.DB) (any, error) { close(started); <-release; return nil, nil })
	}()
	<-started
	var executed atomic.Bool
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	_, err := p.Do(ctx, true, func(context.Context, *sql.DB) (any, error) { executed.Store(true); return nil, nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	close(release)
	<-done
	p.Do(context.Background(), true, func(context.Context, *sql.DB) (any, error) { return nil, nil })
	if executed.Load() {
		t.Fatal("expired job executed")
	}
}
func TestConcurrentClose(t *testing.T) {
	p := testPool(t)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 300; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := p.Do(ctx, true, func(context.Context, *sql.DB) (any, error) { return 1, nil })
			if err != nil && !errors.Is(err, ErrClosed) {
				t.Errorf("submit: %v", err)
			}
		}()
	}
	close(start)
	closed := make(chan struct{})
	go func() { p.Close(); close(closed) }()
	wg.Wait()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocked")
	}
	if _, err := p.Do(context.Background(), false, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("after close: %v", err)
	}
}
func TestReaderParallelismAndSingleWriter(t *testing.T) {
	p := testPool(t)
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Do(context.Background(), false, func(context.Context, *sql.DB) (any, error) { entered <- struct{}{}; <-release; return nil, nil })
		}()
	}
	for i := 0; i < 3; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("readers serialized")
		}
	}
	close(release)
	wg.Wait()
	var active, peak atomic.Int32
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Do(context.Background(), true, func(context.Context, *sql.DB) (any, error) {
				n := active.Add(1)
				for {
					old := peak.Load()
					if n <= old || peak.CompareAndSwap(old, n) {
						break
					}
				}
				time.Sleep(time.Microsecond)
				active.Add(-1)
				return nil, nil
			})
		}()
	}
	wg.Wait()
	if peak.Load() != 1 {
		t.Fatalf("writers peak %d", peak.Load())
	}
}
func TestWorkerPanicDoesNotKillPool(t *testing.T) {
	p := testPool(t)
	_, err := p.Do(context.Background(), true, func(context.Context, *sql.DB) (any, error) { panic("test") })
	if err == nil {
		t.Fatal("panic swallowed")
	}
	v, err := p.Do(context.Background(), true, func(context.Context, *sql.DB) (any, error) { return 42, nil })
	if err != nil || v != 42 {
		t.Fatalf("pool unusable: %v %v", v, err)
	}
}

func TestUniqueReplies(t *testing.T) {
	p := testPool(t)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			v, err := p.Do(context.Background(), false, func(context.Context, *sql.DB) (any, error) { return n, nil })
			if err != nil || v != n {
				t.Errorf("reply for %d: %v %v", n, v, err)
			}
		}(i)
	}
	wg.Wait()
}
