package deleter

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/iriscript/url-shortener/internal/repository"
)

const (
	taskBufferSize = 1024
	batchSize      = 100
	flushInterval  = 10 * time.Second
	flushTimeout   = 5 * time.Second
)

type Repository interface {
	MarkDeleted(ctx context.Context, records []repository.DeleteRecord) error
}

type Deleter struct {
	repo          Repository
	tasks         chan repository.DeleteRecord
	done          chan struct{}
	batchSize     int
	flushInterval time.Duration
	flushTimeout  time.Duration

	mu     sync.RWMutex
	closed bool
}

func New(repo Repository) *Deleter {
	return newWithConfig(repo, taskBufferSize, batchSize, flushInterval, flushTimeout)
}

func newWithConfig(repo Repository, bufferSize, batch int, interval, timeout time.Duration) *Deleter {
	d := &Deleter{
		repo:          repo,
		tasks:         make(chan repository.DeleteRecord, bufferSize),
		done:          make(chan struct{}),
		batchSize:     batch,
		flushInterval: interval,
		flushTimeout:  timeout,
	}

	go d.run()

	return d
}

func (d *Deleter) Enqueue(userID string, ids []string) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return
	}

	for _, id := range ids {
		if id == "" {
			continue
		}

		d.tasks <- repository.DeleteRecord{ID: id, UserID: userID}
	}
}

func (d *Deleter) run() {
	ticker := time.NewTicker(d.flushInterval)
	defer ticker.Stop()

	buf := make([]repository.DeleteRecord, 0, d.batchSize)

	for {
		select {
		case rec, ok := <-d.tasks:
			if !ok {
				d.flush(buf)
				close(d.done)
				return
			}

			buf = append(buf, rec)
			if len(buf) >= d.batchSize {
				buf = d.flush(buf)
			}

		case <-ticker.C:
			buf = d.flush(buf)
		}
	}
}

func (d *Deleter) flush(buf []repository.DeleteRecord) []repository.DeleteRecord {
	if len(buf) == 0 {
		return buf
	}

	ctx, cancel := context.WithTimeout(context.Background(), d.flushTimeout)
	defer cancel()

	if err := d.repo.MarkDeleted(ctx, buf); err != nil {
		log.Error().Err(err).Int("count", len(buf)).Msg("deleter: failed to flush the delete batch")
	}

	return buf[:0]
}

func (d *Deleter) Stop(ctx context.Context) error {
	d.mu.Lock()
	d.closed = true
	close(d.tasks)
	d.mu.Unlock()

	select {
	case <-d.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
