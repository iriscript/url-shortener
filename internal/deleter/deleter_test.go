package deleter

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iriscript/url-shortener/internal/config"
	"github.com/iriscript/url-shortener/internal/model"
)

type fakeRepo struct {
	mu      sync.Mutex
	batches [][]model.DeleteRecord
	calls   chan []model.DeleteRecord
	gate    chan struct{}
	delay   time.Duration

	inFlight    atomic.Int32
	maxInFlight atomic.Int32
}

func (f *fakeRepo) MarkDeleted(ctx context.Context, records []model.DeleteRecord) error {
	for {
		now := f.inFlight.Add(1)
		top := f.maxInFlight.Load()
		if now <= top || f.maxInFlight.CompareAndSwap(top, now) {
			break
		}
		f.inFlight.Add(-1)
	}
	defer f.inFlight.Add(-1)

	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	batch := append([]model.DeleteRecord(nil), records...)

	f.mu.Lock()
	f.batches = append(f.batches, batch)
	f.mu.Unlock()

	if f.calls != nil {
		f.calls <- batch
	}

	return nil
}

func (f *fakeRepo) recorded() [][]model.DeleteRecord {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([][]model.DeleteRecord(nil), f.batches...)
}

func singleWorker(batch int, interval, timeout time.Duration) config.DeleterConfig {
	return config.DeleterConfig{
		Workers:       1,
		MaxWriters:    4,
		TaskBuffer:    10,
		BatchSize:     batch,
		FlushInterval: interval,
		FlushTimeout:  timeout,
	}
}

func stalledRepo() (*fakeRepo, func()) {
	repo := &fakeRepo{gate: make(chan struct{})}

	var once sync.Once
	release := func() { once.Do(func() { close(repo.gate) }) }

	return repo, release
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
	repo := &fakeRepo{calls: make(chan []model.DeleteRecord, 1)}
	d := New(repo, singleWorker(3, time.Hour, time.Second))
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
	repo := &fakeRepo{calls: make(chan []model.DeleteRecord, 1)}
	d := New(repo, singleWorker(100, 20*time.Millisecond, time.Second))
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
	repo := &fakeRepo{calls: make(chan []model.DeleteRecord, 1)}
	d := New(repo, singleWorker(1, time.Hour, time.Second))
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
	repo := &fakeRepo{calls: make(chan []model.DeleteRecord, 1)}
	d := New(repo, singleWorker(100, time.Hour, time.Second))

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
	d := New(repo, singleWorker(100, time.Hour, time.Second))

	d.Enqueue("user1", []string{"a"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := d.Stop(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error = %v, want context.DeadlineExceeded", err)
	}
}

func TestDeleter_EnqueueDoesNotBlockOnAStalledPipeline(t *testing.T) {
	const budget = 50 * time.Millisecond

	repo, release := stalledRepo()
	defer release()

	d := New(repo, config.DeleterConfig{
		Workers:       1,
		MaxWriters:    4,
		TaskBuffer:    1,
		BatchSize:     1,
		FlushInterval: time.Hour,
		FlushTimeout:  10 * time.Second,
	})
	stopDeleter(t, d)

	ids := make([]string, 0, 64)
	for i := range 64 {
		ids = append(ids, fmt.Sprintf("id%02d", i))
	}

	start := time.Now()
	d.Enqueue("user1", ids)
	elapsed := time.Since(start)

	if elapsed > budget {
		t.Errorf("Enqueue took %v with the repository stalled, want under %v", elapsed, budget)
	}
}

func TestDeleter_LimitsWriterGoroutines(t *testing.T) {
	const maxWriters = 2

	repo, release := stalledRepo()
	defer release()

	d := New(repo, config.DeleterConfig{
		Workers:       1,
		MaxWriters:    maxWriters,
		TaskBuffer:    1,
		BatchSize:     1,
		FlushInterval: time.Hour,
		FlushTimeout:  10 * time.Second,
	})
	stopDeleter(t, d)

	for w := range maxWriters {
		ids := make([]string, 0, 20)
		for i := range 20 {
			ids = append(ids, fmt.Sprintf("w%02di%02d", w, i))
		}

		d.Enqueue("user1", ids)
	}

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		d.Enqueue("user1", []string{"overflow"})
	}()

	select {
	case <-returned:
		t.Fatalf("Enqueue returned while %d writers were already in flight, want it to wait for a slot", maxWriters)
	case <-time.After(200 * time.Millisecond):
	}

	release()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue did not return after the writers were released")
	}
}

func TestDeleter_StopWaitsForWriters(t *testing.T) {
	const (
		requests   = 8
		perRequest = 5
	)

	repo := &fakeRepo{}
	d := New(repo, config.DeleterConfig{
		Workers:       2,
		MaxWriters:    4,
		TaskBuffer:    1,
		BatchSize:     3,
		FlushInterval: time.Hour,
		FlushTimeout:  2 * time.Second,
	})

	want := make(map[string]struct{}, requests*perRequest)
	for r := range requests {
		ids := make([]string, 0, perRequest)
		for i := range perRequest {
			id := fmt.Sprintf("r%02di%02d", r, i)
			ids = append(ids, id)
			want[id] = struct{}{}
		}

		d.Enqueue("user1", ids)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	got := make(map[string]struct{}, len(want))
	for _, batch := range repo.recorded() {
		for _, rec := range batch {
			got[rec.ID] = struct{}{}
		}
	}

	if len(got) != len(want) {
		t.Fatalf("repository saw %d records, want %d", len(got), len(want))
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("record %q never reached the repository", id)
		}
	}
}

func TestDeleter_WorkersFlushInParallel(t *testing.T) {
	const workers = 4

	repo := &fakeRepo{delay: 100 * time.Millisecond}
	d := New(repo, config.DeleterConfig{
		Workers:       workers,
		MaxWriters:    4,
		TaskBuffer:    1,
		BatchSize:     1,
		FlushInterval: time.Hour,
		FlushTimeout:  time.Second,
	})

	d.Enqueue("user1", []string{"a", "b", "c", "d"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if got := repo.maxInFlight.Load(); got < 2 {
		t.Errorf("peak concurrent flushes = %d, want at least 2 with %d workers", got, workers)
	}
}

func TestDeleter_BatchesDoNotShareBuffer(t *testing.T) {
	repo := &fakeRepo{delay: 50 * time.Millisecond}
	d := New(repo, config.DeleterConfig{
		Workers:       1,
		MaxWriters:    4,
		TaskBuffer:    1,
		BatchSize:     2,
		FlushInterval: time.Hour,
		FlushTimeout:  time.Second,
	})

	d.Enqueue("user1", []string{"a", "b", "c", "d"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	var got []string
	for _, batch := range repo.recorded() {
		for _, rec := range batch {
			got = append(got, rec.ID)
		}
	}

	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("workers saw %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("workers saw %v, want %v", got, want)
		}
	}
}
