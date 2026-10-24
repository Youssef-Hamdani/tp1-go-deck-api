// Package handlers implements the assignment's GET API and middleware chain.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/database"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/models"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type requestKey struct{}

var sequence atomic.Uint64
var processPrefix = strconv.FormatInt(time.Now().UnixNano(), 36)

type API struct {
	Store   *database.Store
	Timeout time.Duration
	BaseURL string
	Assets  http.Handler
	history sync.WaitGroup
	life    sync.RWMutex
	closed  bool
}

func New(store *database.Store, timeout time.Duration, base string, assets http.Handler) *API {
	return &API{Store: store, Timeout: timeout, BaseURL: strings.TrimRight(base, "/"), Assets: assets}
}
func (a *API) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/deck/new/{$}", a.create)
	mux.HandleFunc("GET /api/deck/{deck_id}/shuffle/{$}", a.deck)
	mux.HandleFunc("GET /api/deck/{deck_id}/draw/{$}", a.deck)
	mux.HandleFunc("GET /api/deck/{deck_id}/return/{$}", a.deck)
	for _, op := range []string{"add", "list", "shuffle", "return", "draw"} {
		mux.HandleFunc("GET /api/deck/{deck_id}/pile/{pile_name}/"+op+"/{$}", a.pile)
	}
	for _, op := range []string{"top", "bottom", "random"} {
		mux.HandleFunc("GET /api/deck/{deck_id}/pile/{pile_name}/draw/"+op+"/{$}", a.pile)
	}
	mux.Handle("GET /static/img/", a.Assets)
	return a.recover(a.deadline(a.requestID(mux)))
}
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.Router().ServeHTTP(w, r) }
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("response: %v", err)
	}
}
func fail(w http.ResponseWriter, id string, err error) {
	status := 500
	msg := "Database error"
	var apiErr *models.APIError
	if errors.As(err, &apiErr) {
		status = apiErr.Status
		msg = apiErr.Message
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, database.ErrClosed) {
		status = 503
		msg = "Request timeout"
	} else {
		log.Printf("database: %v", err)
	}
	writeJSON(w, status, struct {
		Success bool   `json:"success"`
		DeckID  string `json:"deck_id"`
		Error   string `json:"error"`
	}{false, id, msg})
}
func (a *API) base(r *http.Request) string {
	if a.BaseURL != "" {
		return a.BaseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
func (a *API) known(id string, pile bool) error {
	n, ok := a.Store.Count(id)
	if !ok {
		return models.Fail("Deck not found", 404)
	}
	if pile && n > 1 {
		return models.Fail("Pile operations not supported with multiple decks", 400)
	}
	return nil
}
func (a *API) finish(w http.ResponseWriter, r *http.Request, v any, err error, op string) {
	if err != nil {
		fail(w, r.PathValue("deck_id"), err)
		return
	}
	result := v.(models.Response)
	writeJSON(w, 200, result)
	if op != "" {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		a.record(r.Context(), op, result.DeckID)
	}
}
func (a *API) record(parent context.Context, op, id string) {
	// Shutdown takes life exclusively before Wait, preventing Add/Wait races.
	a.life.RLock()
	defer a.life.RUnlock()
	if a.closed {
		return
	}
	a.history.Add(1)
	go func() {
		defer a.history.Done()
		defer func() {
			if v := recover(); v != nil {
				log.Printf("history panic: %v", v)
			}
		}()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer cancel()
		rid, _ := ctx.Value(requestKey{}).(string)
		if err := a.Store.History(ctx, rid, op, id); err != nil {
			log.Printf("history: %v", err)
		}
	}()
}

// Close must follow HTTP Server.Shutdown; the pool remains alive until history drains.
func (a *API) Close() { a.life.Lock(); a.closed = true; a.life.Unlock(); a.history.Wait() }
func (a *API) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic: %v", v)
				fail(w, r.PathValue("deck_id"), models.Fail("Internal server error", 500))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func (a *API) deadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), a.Timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (a *API) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := processPrefix + "-" + strconv.FormatUint(sequence.Add(1), 10)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestKey{}, id)))
	})
}
