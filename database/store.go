package database

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/models"
	"math/rand/v2"
	"sync"
	"time"
)

type Store struct {
	Pool  *WorkerPool
	mu    sync.RWMutex
	decks map[string]int
}

func NewStore(pool *WorkerPool) (*Store, error) {
	s := &Store{Pool: pool, decks: make(map[string]int)}
	_, err := pool.Do(context.Background(), false, func(ctx context.Context, db *sql.DB) (any, error) {
		rows, err := db.QueryContext(ctx, "SELECT id,deck_count FROM decks")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				return nil, err
			}
			s.decks[id] = n
		}
		return nil, rows.Err()
	})
	return s, err
}
func (s *Store) Count(id string) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.decks[id]
	return n, ok
}
func (s *Store) publish(id string, n int) { s.mu.Lock(); defer s.mu.Unlock(); s.decks[id] = n }
func load(ctx context.Context, tx *sql.Tx, id string) (*models.Deck, error) {
	d := &models.Deck{ID: id, Piles: map[string]bool{}}
	if err := tx.QueryRowContext(ctx, "SELECT deck_count,shuffled FROM decks WHERE id=?", id).Scan(&d.Count, &d.Shuffled); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,code,location,pile,position FROM cards WHERE deck_id=? ORDER BY id", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c models.PhysicalCard
		if err := rows.Scan(&c.ID, &c.Code, &c.Location, &c.Pile, &c.Position); err != nil {
			rows.Close()
			return nil, err
		}
		d.Cards = append(d.Cards, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT name FROM piles WHERE deck_id=?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		d.Piles[name] = true
	}
	return d, rows.Err()
}

// Operate executes the whole state transition within one job and transaction.
// The snapshot is local to this job: cards/slices are never shared across requests.
func (s *Store) Operate(ctx context.Context, id string, write bool, create *models.Deck, fn func(*models.Deck) (any, error)) (any, error) {
	return s.Pool.Do(ctx, write, func(ctx context.Context, db *sql.DB) (any, error) {
		tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: !write})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		var d *models.Deck
		if create != nil {
			d = create
			for {
				b := make([]byte, 12)
				const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
				for i := range b {
					b[i] = alphabet[rand.IntN(len(alphabet))]
				}
				d.ID = string(b)
				var exists int
				err = tx.QueryRowContext(ctx, "SELECT count(*) FROM decks WHERE id=?", d.ID).Scan(&exists)
				if err != nil {
					return nil, err
				}
				if exists == 0 {
					break
				}
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO decks(id,deck_count,shuffled) VALUES(?,?,?)", d.ID, d.Count, d.Shuffled); err != nil {
				return nil, err
			}
		} else {
			d, err = load(ctx, tx, id)
			if err != nil {
				return nil, err
			}
		}
		before := append([]models.PhysicalCard(nil), d.Cards...)
		oldPiles := map[string]bool{}
		for k, v := range d.Piles {
			oldPiles[k] = v
		}
		result, err := fn(d)
		if err != nil {
			return nil, err
		}
		if write {
			if _, err = tx.ExecContext(ctx, "UPDATE decks SET shuffled=? WHERE id=?", d.Shuffled, d.ID); err != nil {
				return nil, err
			}
			for name := range d.Piles {
				if !oldPiles[name] {
					if _, err = tx.ExecContext(ctx, "INSERT INTO piles(deck_id,name) VALUES(?,?)", d.ID, name); err != nil {
						return nil, err
					}
				}
			}
			query := "UPDATE cards SET location=?,pile=?,position=? WHERE deck_id=? AND id=?"
			if create != nil {
				query = "INSERT INTO cards(location,pile,position,deck_id,id,code) VALUES(?,?,?,?,?,?)"
			}
			stmt, err := tx.PrepareContext(ctx, query)
			if err != nil {
				return nil, err
			}
			defer stmt.Close()
			for i, c := range d.Cards {
				if create == nil && c == before[i] {
					continue
				}
				args := []any{c.Location, c.Pile, c.Position, d.ID, c.ID}
				if create != nil {
					args = append(args, c.Code)
				}
				if _, err = stmt.ExecContext(ctx, args...); err != nil {
					return nil, err
				}
			}
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		// Publish inside the writer job after commit, even if the waiting client timed out.
		if create != nil {
			s.publish(d.ID, d.Count)
		}
		return result, nil
	})
}
func (s *Store) History(ctx context.Context, requestID, operation, id string) error {
	_, err := s.Pool.Do(ctx, true, func(ctx context.Context, db *sql.DB) (any, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		_, err = tx.ExecContext(ctx, "INSERT INTO historique(horodatage,request_id,operation,deck_id) VALUES(?,?,?,?)", time.Now().UTC().Format(time.RFC3339Nano), requestID, operation, id)
		if err != nil {
			return nil, err
		}
		return nil, tx.Commit()
	})
	return err
}
func (s *Store) Check(ctx context.Context) (string, error) {
	v, err := s.Pool.Do(ctx, false, func(ctx context.Context, db *sql.DB) (any, error) {
		var result string
		err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result)
		return result, err
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprint(v), nil
}
