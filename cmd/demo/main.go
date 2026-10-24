// Command demo exercises a running server and checks concurrent card integrity.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/models"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/utils"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type reply struct {
	models.Response
	Error string `json:"error,omitempty"`
}

var client = &http.Client{Timeout: 10 * time.Second}

func get(base, path string) (reply, int, error) {
	resp, err := client.Get(base + path)
	if err != nil {
		return reply{}, 0, err
	}
	defer resp.Body.Close()
	var r reply
	err = json.NewDecoder(resp.Body).Decode(&r)
	return r, resp.StatusCode, err
}
func must(base, path string) reply {
	r, status, err := get(base, path)
	if err != nil || status != 200 || !r.Success {
		fmt.Fprintf(os.Stderr, "FAIL %s: %d %+v %v\n", path, status, r, err)
		os.Exit(1)
	}
	b, _ := json.Marshal(r)
	fmt.Printf("GET %s\n%s\n", path, b)
	return r
}
func load(base string, n int) error {
	r := must(base, "/api/deck/new/")
	url := "/api/deck/" + r.DeckID + "/draw/"
	start := make(chan struct{})
	results := make(chan reply, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	begin := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, status, err := get(base, url)
			if err != nil {
				errs <- err
				return
			}
			if status != 200 && !(status == 400 && r.Error == "Deck is empty") {
				errs <- fmt.Errorf("unexpected status %d: %+v", status, r)
				return
			}
			results <- r
		}()
	}
	close(start)
	wg.Wait()
	elapsed := time.Since(begin)
	close(results)
	close(errs)
	for err := range errs {
		return err
	}
	seen := map[string]bool{}
	empty := 0
	for r := range results {
		if !r.Success {
			empty++
			continue
		}
		if r.Cards == nil || len(*r.Cards) != 1 {
			return fmt.Errorf("missing card")
		}
		code := (*r.Cards)[0].Code
		if seen[code] {
			return fmt.Errorf("duplicate card %s", code)
		}
		seen[code] = true
	}
	if len(seen) != 52 || empty != n-52 {
		return fmt.Errorf("integrity: %d distinct cards and %d empty errors", len(seen), empty)
	}
	for _, code := range utils.Standard(false) {
		if !seen[code] {
			return fmt.Errorf("missing %s", code)
		}
	}
	restored := must(base, "/api/deck/"+r.DeckID+"/return/")
	if restored.Remaining != 52 {
		return fmt.Errorf("lost cards: remaining=%d", restored.Remaining)
	}
	fmt.Printf("PASS: %d concurrent requests in %s = %.2f requests/sec; 52 distinct cards, %d empty responses; return restores 52. Async history enabled.\n", n, elapsed, float64(n)/elapsed.Seconds(), empty)
	return nil
}
func main() {
	baseFlag := flag.String("base", "http://localhost:8080", "server URL")
	loadOnly := flag.Bool("load-only", false, "only concurrent integrity demo")
	n := flag.Int("requests", 104, "concurrent requests (52..1000)")
	flag.Parse()
	base := strings.TrimRight(*baseFlag, "/")
	if *n < 52 || *n > 1000 {
		fmt.Fprintln(os.Stderr, "requests must be 52..1000")
		os.Exit(1)
	}
	if !*loadOnly {
		for _, url := range []string{"/api/deck/new/", "/api/deck/new/shuffle/", "/api/deck/new/?deck_count=6", "/api/deck/new/?jokers_enabled=true", "/api/deck/new/shuffle/?cards=AS,2S,KS,AD,2D,KD,AC,2C,KC,AH,2H,KH", "/api/deck/new/draw/?count=2"} {
			must(base, url)
		}
		r := must(base, "/api/deck/new/")
		p := "/api/deck/" + r.DeckID + "/"
		for _, op := range []string{"draw/?count=5", "pile/discard/add/?cards=AS,2S,3S,4S,5S", "pile/discard/list/", "pile/discard/draw/", "pile/discard/draw/bottom/", "pile/discard/shuffle/", "pile/discard/draw/random/", "pile/discard/return/", "return/", "shuffle/?remaining=true", "shuffle/"} {
			must(base, p+op)
		}
		for _, tt := range []struct {
			path, msg string
			status    int
		}{{"/api/deck/unknown/draw/", "Deck not found", 404}, {p + "draw/?count=53", "Not enough cards remaining", 400}, {p + "pile/discard/draw/", "Pile is empty", 400}} {
			r, status, err := get(base, tt.path)
			if err != nil || status != tt.status || r.Error != tt.msg {
				fmt.Fprintln(os.Stderr, "error contract failed", r, status, err)
				os.Exit(1)
			}
			fmt.Printf("PASS expected error %d: %s\n", status, r.Error)
		}
		for _, code := range []string{"AS", "10H", "X1", "X2", "back"} {
			resp, err := client.Get(base + "/static/img/" + code + ".svg")
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			resp.Body.Close()
			if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/svg+xml") {
				fmt.Fprintln(os.Stderr, "invalid SVG", code)
				os.Exit(1)
			}
			fmt.Printf("PASS SVG %s/static/img/%s.svg\n", base, code)
		}
	}
	if err := load(base, *n); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
