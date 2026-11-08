package deleter

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/iriscript/url-shortener/internal/config"
	"github.com/iriscript/url-shortener/internal/model"
)

type Repository interface {
	MarkDeleted(ctx context.Context, records []model.DeleteRecord) error
}

type Deleter struct {
	repo    Repository
	cfg     config.DeleterConfig
	tasks   chan model.DeleteRecord
	batches chan []model.DeleteRecord
	slots   chan struct{}
	done    chan struct{}
	wg      sync.WaitGroup
	writers sync.WaitGroup

	mu     sync.Mutex
	closed bool
}

func New(repo Repository, cfg config.DeleterConfig) *Deleter {
	d := &Deleter{
		repo:    repo,
		cfg:     cfg,
		tasks:   make(chan model.DeleteRecord, cfg.TaskBuffer),
		batches: make(chan []model.DeleteRecord, cfg.Workers),
		slots:   make(chan struct{}, cfg.MaxWriters),
		done:    make(chan struct{}),
	}

	d.wg.Add(cfg.Workers)
	for range cfg.Workers {
		go d.run()
	}

	go d.batch()

	go func() {
		d.wg.Wait()
		close(d.done)
	}()

	return d
}

func (d *Deleter) Enqueue(userID string, ids []string) {
	records := make([]model.DeleteRecord, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}

		records = append(records, model.DeleteRecord{ID: id, UserID: userID})
	}

	if len(records) == 0 {
		return
	}

	d.slots <- struct{}{}

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		<-d.slots

		return
	}
	d.writers.Add(1)
	d.mu.Unlock()

	go d.write(records)
}

func (d *Deleter) write(records []model.DeleteRecord) {
	defer d.writers.Done()
	defer func() { <-d.slots }()

	for _, rec := range records {
		d.tasks <- rec
	}
}

func (d *Deleter) batch() {
	ticker := time.NewTicker(d.cfg.FlushInterval)
	defer ticker.Stop()
	defer close(d.batches)

	buf := make([]model.DeleteRecord, 0, d.cfg.BatchSize)

	for {
		select {
		case rec, ok := <-d.tasks:
			if !ok {
				d.send(buf)
				return
			}

			buf = append(buf, rec)
			if len(buf) >= d.cfg.BatchSize {
				buf = d.send(buf)
			}

		case <-ticker.C:
			buf = d.send(buf)
		}
	}
}

func (d *Deleter) send(buf []model.DeleteRecord) []model.DeleteRecord {
	if len(buf) == 0 {
		return buf
	}

	d.batches <- buf

	return make([]model.DeleteRecord, 0, d.cfg.BatchSize)
}

func (d *Deleter) run() {
	defer d.wg.Done()

	for buf := range d.batches {
		d.flush(buf)
	}
}

func (d *Deleter) flush(buf []model.DeleteRecord) {
	ctx, cancel := context.WithTimeout(context.Background(), d.cfg.FlushTimeout)
	defer cancel()

	if err := d.repo.MarkDeleted(ctx, buf); err != nil {
		log.Error().Err(err).Int("count", len(buf)).Msg("deleter: failed to flush the delete batch")
	}
}

func (d *Deleter) Stop(ctx context.Context) error {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()

	drained := make(chan struct{})
	go func() {
		d.writers.Wait()
		close(drained)
	}()

	select {
	case <-drained:
	case <-ctx.Done():
		return ctx.Err()
	}

	close(d.tasks)

	select {
	case <-d.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
