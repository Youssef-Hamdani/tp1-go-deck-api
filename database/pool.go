// Package database owns SQLite and its bounded reader/single-writer worker pool.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"sync"
)

var ErrClosed = errors.New("worker pool closed")

type Result struct {
	Data any
	Err  error
}
type job struct {
	ctx   context.Context
	run   func(context.Context, *sql.DB) (any, error)
	reply chan Result
}
type WorkerPool struct {
	db            *sql.DB
	reads, writes chan job
	gate          sync.RWMutex // excludes channel closure from all pending sends, not job execution.
	closed        bool
	stop          chan struct{}
	workers       sync.WaitGroup
	once          sync.Once
}

func Open(path string, readers int) (*WorkerPool, error) {
	if readers < 1 || readers > 3 {
		return nil, fmt.Errorf("readers must be 1..3")
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(readers + 1)
	db.SetMaxIdleConns(readers + 1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	p := &WorkerPool{db: db, reads: make(chan job, 256), writes: make(chan job, 256), stop: make(chan struct{})}
	for i := 0; i < readers; i++ {
		p.workers.Add(1)
		go p.worker(p.reads)
	}
	p.workers.Add(1)
	go p.worker(p.writes)
	return p, nil
}
func (p *WorkerPool) worker(jobs <-chan job) {
	defer p.workers.Done()
	for j := range jobs {
		j.reply <- p.execute(j)
	}
}
func (p *WorkerPool) execute(j job) (res Result) {
	defer func() {
		if v := recover(); v != nil {
			res = Result{Err: fmt.Errorf("worker panic: %v", v)}
		}
	}()
	if err := j.ctx.Err(); err != nil {
		return Result{Err: err}
	}
	res.Data, res.Err = j.run(j.ctx, p.db)
	return
}
func (p *WorkerPool) Do(ctx context.Context, write bool, run func(context.Context, *sql.DB) (any, error)) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	j := job{ctx: ctx, run: run, reply: make(chan Result, 1)}
	ch := p.reads
	if write {
		ch = p.writes
	}
	p.gate.RLock()
	if p.closed {
		p.gate.RUnlock()
		return nil, ErrClosed
	}
	select {
	case ch <- j:
	case <-ctx.Done():
		p.gate.RUnlock()
		return nil, ctx.Err()
	case <-p.stop:
		p.gate.RUnlock()
		return nil, ErrClosed
	}
	p.gate.RUnlock()
	select {
	case res := <-j.reply:
		return res.Data, res.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (p *WorkerPool) Close() error {
	p.once.Do(func() {
		close(p.stop)
		p.gate.Lock()
		p.closed = true
		close(p.reads)
		close(p.writes)
		p.gate.Unlock()
		p.workers.Wait()
		p.db.Close()
	})
	return nil
}

const schema = `
CREATE TABLE IF NOT EXISTS decks(id TEXT PRIMARY KEY, deck_count INTEGER NOT NULL CHECK(deck_count BETWEEN 1 AND 20), shuffled INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS piles(deck_id TEXT NOT NULL REFERENCES decks(id), name TEXT NOT NULL, PRIMARY KEY(deck_id,name));
CREATE TABLE IF NOT EXISTS cards(deck_id TEXT NOT NULL REFERENCES decks(id), id INTEGER NOT NULL, code TEXT NOT NULL, location TEXT NOT NULL CHECK(location IN ('deck','drawn','pile')), pile TEXT NOT NULL DEFAULT '', position INTEGER NOT NULL, PRIMARY KEY(deck_id,id), CHECK((location='pile' AND pile<>'') OR (location<>'pile' AND pile='')));
CREATE INDEX IF NOT EXISTS cards_location ON cards(deck_id,location,pile,position,id);
CREATE TABLE IF NOT EXISTS historique(id INTEGER PRIMARY KEY, horodatage TEXT NOT NULL, request_id TEXT NOT NULL, operation TEXT NOT NULL, deck_id TEXT NOT NULL REFERENCES decks(id));
CREATE INDEX IF NOT EXISTS historique_deck ON historique(deck_id,id);
`
