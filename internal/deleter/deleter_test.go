package deleter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/iriscript/url-shortener/internal/repository"
)

type fakeRepo struct {
	mu      sync.Mutex
	batches [][]repository.DeleteRecord
	calls   chan []repository.DeleteRecord
	delay   time.Duration
}

func (f *fakeRepo) MarkDeleted(ctx context.Context, records []repository.DeleteRecord) error {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	batch := append([]repository.DeleteRecord(nil), records...)

	f.mu.Lock()
	f.batches = append(f.batches, batch)
	f.mu.Unlock()

	if f.calls != nil {
		f.calls <- batch
	}

	return nil
}

func stopDeleter(t *testing.T, d *Deleter) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = d.Stop(ctx)
	})
}

func TestDeleter_FlushesOnBatchSize(t *testing.T) {
	repo := &fakeRepo{calls: make(chan []repository.DeleteRecord, 1)}
	d := newWithConfig(repo, 10, 3, time.Hour, time.Second)
	stopDeleter(t, d)

	d.Enqueue("user1", []string{"a", "b", "c"})

	select {
	case batch := <-repo.calls:
		if len(batch) != 3 {
			t.Fatalf("got batch of %d records, want 3", len(batch))
		}
	case <-time.After(time.Second):
		t.Fatal("flush did not happen in time")
	}
}

func TestDeleter_FlushesOnTimer(t *testing.T) {
	repo := &fakeRepo{calls: make(chan []repository.DeleteRecord, 1)}
	d := newWithConfig(repo, 10, 100, 20*time.Millisecond, time.Second)
	stopDeleter(t, d)

	d.Enqueue("user1", []string{"a"})

	select {
	case batch := <-repo.calls:
		if len(batch) != 1 {
			t.Fatalf("got batch of %d records, want 1", len(batch))
		}
	case <-time.After(time.Second):
		t.Fatal("flush did not happen in time")
	}
}

func TestDeleter_EnqueueSkipsEmptyIDs(t *testing.T) {
	repo := &fakeRepo{calls: make(chan []repository.DeleteRecord, 1)}
	d := newWithConfig(repo, 10, 1, time.Hour, time.Second)
	stopDeleter(t, d)

	d.Enqueue("user1", []string{"", "a"})

	select {
	case batch := <-repo.calls:
		if len(batch) != 1 || batch[0].ID != "a" {
			t.Fatalf("got batch %+v, want a single record with ID %q", batch, "a")
		}
	case <-time.After(time.Second):
		t.Fatal("flush did not happen in time")
	}
}

func TestDeleter_StopFlushesRemaining(t *testing.T) {
	repo := &fakeRepo{calls: make(chan []repository.DeleteRecord, 1)}
	d := newWithConfig(repo, 10, 100, time.Hour, time.Second)

	d.Enqueue("user1", []string{"a", "b"})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	select {
	case batch := <-repo.calls:
		if len(batch) != 2 {
			t.Fatalf("got batch of %d records, want 2", len(batch))
		}
	default:
		t.Fatal("Stop returned without flushing the buffered records")
	}
}

func TestDeleter_StopRespectsContextDeadline(t *testing.T) {
	repo := &fakeRepo{delay: 200 * time.Millisecond}
	d := newWithConfig(repo, 10, 100, time.Hour, time.Second)

	d.Enqueue("user1", []string{"a"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := d.Stop(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error = %v, want context.DeadlineExceeded", err)
	}
}
