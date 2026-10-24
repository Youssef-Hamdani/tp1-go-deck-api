package main

import (
	"context"
	"errors"
	"flag"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/database"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/handlers"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/static"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	path := flag.String("db", "deck.db", "SQLite database file")
	readers := flag.Int("readers", 3, "reader workers (1..3)")
	timeout := flag.Duration("timeout", 2*time.Second, "request timeout")
	base := flag.String("base-url", "", "public image URL prefix (default request origin)")
	flag.Parse()
	if *timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	pool, err := database.Open(*path, *readers)
	if err != nil {
		return err
	}
	defer pool.Close()
	store, err := database.NewStore(pool)
	if err != nil {
		return err
	}
	api := handlers.New(store, *timeout, *base, static.Handler())
	defer api.Close()
	server := &http.Server{Addr: *addr, Handler: api.Router(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := make(chan error, 1)
	go func() { log.Printf("Deck API listening on %s", *addr); result <- server.ListenAndServe() }()
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		server.Close()
		return err
	}
	return nil
}
