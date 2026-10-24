package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/database"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/models"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/static"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/utils"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fixture struct {
	a *API
	h http.Handler
	p *database.WorkerPool
}

func setup(t testing.TB) fixture {
	t.Helper()
	p, err := database.Open(filepath.Join(t.TempDir(), "test.db"), 3)
	if err != nil {
		t.Fatal(err)
	}
	s, err := database.NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	a := New(s, 2*time.Second, "http://localhost:8080", static.Handler())
	t.Cleanup(func() { a.Close(); p.Close() })
	return fixture{a, a.Router(), p}
}
func request(h http.Handler, path string) (int, map[string]json.RawMessage, models.Response) {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	var raw map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &raw)
	var res models.Response
	json.Unmarshal(w.Body.Bytes(), &res)
	return w.Code, raw, res
}
func ok(t testing.TB, f fixture, path string) models.Response {
	t.Helper()
	status, raw, r := request(f.h, path)
	if status != 200 || !r.Success {
		t.Fatalf("%s: status=%d body=%s", path, status, raw)
	}
	return r
}
func deck(t testing.TB, f fixture, query string) string {
	t.Helper()
	return ok(t, f, "/api/deck/new/"+query).DeckID
}
func path(id, op string) string { return "/api/deck/" + id + "/" + op }
func cards(r models.Response) []string {
	out := []string{}
	if r.Cards != nil {
		for _, c := range *r.Cards {
			out = append(out, c.Code)
		}
	}
	return out
}
func assertCodes(t testing.TB, r models.Response, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(cards(r), want) {
		t.Fatalf("cards %v want %v", cards(r), want)
	}
}
func errorWant(t *testing.T, f fixture, url string, status int, message string) {
	t.Helper()
	got, raw, _ := request(f.h, url)
	var msg string
	json.Unmarshal(raw["error"], &msg)
	if got != status || msg != message || string(raw["success"]) != "false" {
		t.Fatalf("%s got %d %s want %d %q", url, got, raw, status, message)
	}
}
func TestDeckContractAndOrder(t *testing.T) {
	f := setup(t)
	status, raw, r := request(f.h, "/api/deck/new/")
	if status != 200 || len(raw) != 4 || r.Remaining != 52 || r.Shuffled == nil || *r.Shuffled {
		t.Fatalf("new: %d %v", status, raw)
	}
	got := ok(t, f, path(r.DeckID, "draw/?count=52"))
	if got.Shuffled != nil || got.Piles != nil || got.Remaining != 0 {
		t.Fatal(got)
	}
	assertCodes(t, got, utils.Standard(false)...)
	for _, c := range *got.Cards {
		if c.Image != "http://localhost:8080/static/img/"+c.Code+".svg" || c.Value == "ACE" || c.Suit == "SPADES" {
			t.Fatal(c)
		}
	}
	errorWant(t, f, path(r.DeckID, "draw/"), 400, "Deck is empty")
	id := deck(t, f, "")
	errorWant(t, f, path(id, "draw/?count=53"), 400, "Not enough cards remaining")
	if r := ok(t, f, path(id, "draw/")); r.Remaining != 51 {
		t.Fatal(r)
	}
}
func TestCreationVariants(t *testing.T) {
	f := setup(t)
	for _, tt := range []struct {
		url      string
		n        int
		shuffled bool
	}{{"/api/deck/new/?deck_count=6", 312, false}, {"/api/deck/new/?jokers_enabled=true", 54, false}, {"/api/deck/new/shuffle/", 52, true}, {"/api/deck/new/shuffle/?cards=as,0h,kd", 3, true}, {"/api/deck/new/shuffle/?cards=AS,2S&deck_count=2&jokers_enabled=true", 4, true}, {"/api/deck/new/?deck_count=2&jokers_enabled=true", 108, false}} {
		r := ok(t, f, tt.url)
		if r.Remaining != tt.n || r.Shuffled == nil || *r.Shuffled != tt.shuffled {
			t.Fatalf("%s: %+v", tt.url, r)
		}
		got := ok(t, f, path(r.DeckID, fmt.Sprintf("draw/?count=%d", tt.n)))
		if len(cards(got)) != tt.n || got.Remaining != 0 {
			t.Fatal(got)
		}
	}
	r := ok(t, f, "/api/deck/new/draw/?count=2")
	if r.Remaining != 50 || len(cards(r)) != 2 || r.Shuffled != nil {
		t.Fatal(r)
	}
	r = ok(t, f, "/api/deck/new/?jokers_enabled=true")
	drawn := ok(t, f, path(r.DeckID, "draw/?count=54"))
	for _, c := range (*drawn.Cards)[52:] {
		if c.Value != "JOKER" || !(c.Suit == "NOIR" || c.Suit == "ROUGE") {
			t.Fatal(c)
		}
	}
}
func TestPileLifecycle(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "")
	p := func(op string) string { return path(id, "pile/hand/"+op) }
	errorWant(t, f, p("add/?cards=AS"), 404, "Card not found in deck")
	assertCodes(t, ok(t, f, path(id, "draw/?count=5")), "AS", "2S", "3S", "4S", "5S")
	r := ok(t, f, p("add/?cards=AS,2S,3S,4S"))
	if r.Remaining != 47 || r.Piles["hand"].Remaining != 4 || r.Shuffled != nil {
		t.Fatal(r)
	}
	ok(t, f, path(id, "pile/other/add/?cards=5S"))
	r = ok(t, f, p("list/"))
	if r.Piles["other"].Cards != nil || r.Piles["hand"].Cards == nil || r.Piles["hand"].Remaining != 4 {
		t.Fatal(r)
	}
	assertCodes(t, ok(t, f, p("draw/")), "4S")
	assertCodes(t, ok(t, f, p("draw/bottom/")), "AS")
	assertCodes(t, ok(t, f, p("draw/top/")), "3S")
	assertCodes(t, ok(t, f, p("draw/random/")), "2S")
	errorWant(t, f, p("draw/"), 400, "Pile is empty")
	ok(t, f, p("add/?cards=AS,2S,3S,4S"))
	ok(t, f, p("shuffle/"))
	assertCodes(t, ok(t, f, p("draw/?cards=3S,AS")), "3S", "AS")
	r = ok(t, f, p("return/?cards=2S"))
	if r.Remaining != 48 || r.Piles["hand"].Remaining != 1 || r.Shuffled == nil {
		t.Fatal(r)
	}
	ok(t, f, p("return/"))
	r = ok(t, f, path(id, "return/"))
	if r.Remaining != 51 || r.Piles["other"].Remaining != 1 {
		t.Fatal(r)
	}
	r = ok(t, f, path(id, "pile/other/return/"))
	if r.Remaining != 52 {
		t.Fatal(r)
	}
	// Returned cards are appended; the still-undrawn top remains 6S.
	assertCodes(t, ok(t, f, path(id, "draw/")), "6S")
}
func TestShuffleAndReturnSemantics(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "")
	ok(t, f, path(id, "draw/?count=3"))
	ok(t, f, path(id, "pile/p/add/?cards=AS"))
	r := ok(t, f, path(id, "shuffle/?remaining=true"))
	if r.Remaining != 49 || !*r.Shuffled {
		t.Fatal(r)
	}
	r = ok(t, f, path(id, "pile/p/list/"))
	if r.Piles["p"].Remaining != 1 {
		t.Fatal(r)
	}
	r = ok(t, f, path(id, "return/?cards=2S"))
	if r.Remaining != 50 {
		t.Fatal(r)
	}
	r = ok(t, f, path(id, "shuffle/"))
	if r.Remaining != 52 {
		t.Fatal(r)
	}
	r = ok(t, f, path(id, "pile/p/list/"))
	if r.Piles["p"].Remaining != 0 {
		t.Fatal(r)
	}
	// Canonical aliases and duplicate physical instances in multi-decks.
	id = deck(t, f, "?deck_count=2")
	ok(t, f, path(id, "draw/?count=53"))
	r = ok(t, f, path(id, "return/?cards=AS"))
	if r.Remaining != 52 {
		t.Fatal(r)
	}
	r = ok(t, f, path(id, "return/?cards=AS"))
	if r.Remaining != 53 {
		t.Fatal(r)
	}
}
func TestErrors(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "")
	multi := deck(t, f, "?deck_count=2")
	cases := []struct {
		url    string
		status int
		msg    string
	}{
		{"/api/deck/missing/draw/", 404, "Deck not found"},
		{path(id, "draw/?count=53"), 400, "Not enough cards remaining"},
		{path(id, "pile/p/add/?cards=99Z"), 400, "Invalid card code"},
		{path(id, "pile/p/add/?cards=AS,as"), 400, "Duplicate cards in request"},
		{path(id, "pile/p/add/?cards=0H,10H"), 400, "Duplicate cards in request"},
		{path(id, "pile/p/add/?cards=AS"), 404, "Card not found in deck"},
		{path(id, "pile/p/list/"), 404, "Pile not found"},
		{path(multi, "pile/p/list/"), 400, "Pile operations not supported with multiple decks"},
		{path(id, "return/?cards=AS"), 404, "Card not found in deck"},
		{"/api/deck/new/?deck_count=0", 400, "Parameter out of range"},
		{"/api/deck/new/?deck_count=21", 400, "Parameter out of range"},
		{"/api/deck/new/?deck_count=oops", 400, "Invalid parameter"},
		{"/api/deck/new/?jokers_enabled=1", 400, "Invalid parameter"},
		{"/api/deck/new/shuffle/?cards=", 400, "Invalid parameter"},
		{path(id, "shuffle/?remaining=yes"), 400, "Invalid parameter"},
		{path(id, "draw/?count=0"), 400, "Parameter out of range"},
		{path(id, "draw/?count=-1"), 400, "Parameter out of range"},
		{path(id, "draw/?count=1&count=2"), 400, "Invalid parameter"},
		{path(id, "draw/?count="), 400, "Invalid parameter"},
		{path(id, "pile/p/add/"), 400, "Invalid parameter"},
	}
	for _, c := range cases {
		errorWant(t, f, c.url, c.status, c.msg)
	}
	ok(t, f, path(id, "draw/?count=2"))
	ok(t, f, path(id, "pile/p/add/?cards=AS"))
	errorWant(t, f, path(id, "pile/p/draw/?cards=2S"), 404, "Card not found in pile")
	errorWant(t, f, path(id, "pile/p/draw/?cards=AS&count=1"), 400, "Invalid parameter")
	errorWant(t, f, path(id, "pile/p/draw/?count=2"), 400, "Not enough cards remaining")
	ok(t, f, path(id, "pile/p/draw/"))
	errorWant(t, f, path(id, "pile/p/draw/"), 400, "Pile is empty")
}
func TestAtomicFailedTransfer(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "")
	ok(t, f, path(id, "draw/?count=2"))
	errorWant(t, f, path(id, "pile/p/add/?cards=AS,3S"), 404, "Card not found in deck")
	errorWant(t, f, path(id, "pile/p/list/"), 404, "Pile not found")
	r := ok(t, f, path(id, "return/"))
	if r.Remaining != 52 {
		t.Fatal(r)
	}
}
func TestEverySVG(t *testing.T) {
	f := setup(t)
	for _, code := range append(utils.Standard(true), "back") {
		w := httptest.NewRecorder()
		f.h.ServeHTTP(w, httptest.NewRequest("GET", "/static/img/"+code+".svg", nil))
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/svg+xml") {
			t.Fatalf("%s %d %s", code, w.Code, w.Header())
		}
		dec := xml.NewDecoder(w.Body)
		root := false
		for {
			token, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(code, err)
			}
			if e, ok := token.(xml.StartElement); ok && !root {
				root = true
				if e.Name.Local != "svg" || e.Name.Space != "http://www.w3.org/2000/svg" {
					t.Fatal(e)
				}
			}
		}
		if !root {
			t.Fatal("empty SVG")
		}
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, httptest.NewRequest("GET", "/static/img/missing.svg", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func TestConcurrentDraw52(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "")
	start := make(chan struct{})
	out := make(chan models.Response, 52)
	var wg sync.WaitGroup
	for i := 0; i < 52; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			status, _, r := request(f.h, path(id, "draw/"))
			if status != 200 {
				t.Errorf("draw status %d", status)
			}
			out <- r
		}()
	}
	close(start)
	wg.Wait()
	close(out)
	seen := map[string]bool{}
	remaining := map[int]bool{}
	for r := range out {
		if len(cards(r)) != 1 {
			t.Fatal(r)
		}
		c := cards(r)[0]
		if seen[c] {
			t.Fatal("duplicate", c)
		}
		seen[c] = true
		remaining[r.Remaining] = true
	}
	for _, c := range utils.Standard(false) {
		if !seen[c] {
			t.Error("missing", c)
		}
	}
	if len(remaining) != 52 {
		t.Fatal("draw counts not consecutive", remaining)
	}
	r := ok(t, f, path(id, "return/"))
	if r.Remaining != 52 {
		t.Fatal(r)
	}
}
func TestConcurrentLoad104(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "")
	srv := httptest.NewServer(f.h)
	defer srv.Close()
	client := srv.Client()
	client.Timeout = 5 * time.Second
	start := make(chan struct{})
	var wg sync.WaitGroup
	var successes, empty atomic.Int32
	seen := map[string]bool{}
	var mu sync.Mutex
	begin := time.Now()
	for i := 0; i < 104; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := client.Get(srv.URL + path(id, "draw/"))
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			var body struct {
				models.Response
				Error string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if resp.StatusCode == 200 {
				successes.Add(1)
				cs := cards(body.Response)
				if len(cs) != 1 {
					t.Error("missing card")
					return
				}
				mu.Lock()
				if seen[cs[0]] {
					t.Error("duplicate card", cs[0])
				}
				seen[cs[0]] = true
				mu.Unlock()
			} else if resp.StatusCode == 400 && body.Error == "Deck is empty" {
				empty.Add(1)
			} else {
				t.Errorf("unexpected response %d %+v", resp.StatusCode, body)
			}
		}()
	}
	close(start)
	wg.Wait()
	elapsed := time.Since(begin)
	if successes.Load() != 52 || empty.Load() != 52 || len(seen) != 52 {
		t.Fatalf("success=%d empty=%d distinct=%d", successes.Load(), empty.Load(), len(seen))
	}
	t.Logf("104 concurrent HTTP requests: %s, %.2f requests/sec (52 successful draws + 52 empty errors; async history enabled)", elapsed, 104/elapsed.Seconds())
	r := ok(t, f, path(id, "return/"))
	if r.Remaining != 52 {
		t.Fatal("conservation", r)
	}
	integrity, err := f.a.Store.Check(context.Background())
	if err != nil || integrity != "ok" {
		t.Fatal(integrity, err)
	}
}
func TestIsolation(t *testing.T) {
	f := setup(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		id := deck(t, f, "")
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				status, _, r := request(f.h, path(id, "draw/"))
				if status != 200 || len(cards(r)) != 1 || cards(r)[0] != utils.Standard(false)[j] {
					t.Errorf("isolation %s: %+v", id, r)
					return
				}
			}
			status, _, r := request(f.h, path(id, "return/"))
			if status != 200 || r.Remaining != 52 {
				t.Error("isolation return", r)
			}
		}(id)
	}
	wg.Wait()
}
func TestTimeoutQueuedJob(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "")
	f.a.history.Wait()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		f.p.Do(context.Background(), true, func(context.Context, *sql.DB) (any, error) { close(started); <-release; return nil, nil })
	}()
	<-started
	f.a.Timeout = 10 * time.Millisecond
	f.h = f.a.Router()
	errorWant(t, f, path(id, "draw/"), 503, "Request timeout")
	close(release)
	<-done
	f.a.Timeout = 2 * time.Second
	f.h = f.a.Router()
	assertCodes(t, ok(t, f, path(id, "draw/")), "AS")
}
func TestMiddlewareAndHistory(t *testing.T) {
	f := setup(t)
	panicHandler := f.a.recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("intentional") }))
	w := httptest.NewRecorder()
	panicHandler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 500 || !strings.Contains(w.Body.String(), "Internal server error") {
		t.Fatal(w)
	}
	var wg sync.WaitGroup
	ids := sync.Map{}
	h := f.a.recover(f.a.deadline(f.a.requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := r.Context().Value(requestKey{}).(string)
		if id == "" {
			t.Error("missing typed ID")
		}
		if _, loaded := ids.LoadOrStore(id, true); loaded {
			t.Error("duplicate ID")
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Error("deadline missing")
		}
	}))))
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil)) }()
	}
	wg.Wait()
	id := deck(t, f, "")
	ok(t, f, path(id, "draw/"))
	f.a.history.Wait()
	_, err := f.p.Do(context.Background(), false, func(ctx context.Context, db *sql.DB) (any, error) {
		var count, unique int
		err := db.QueryRowContext(ctx, "SELECT count(*),count(DISTINCT request_id) FROM historique WHERE deck_id=? AND request_id<>''", id).Scan(&count, &unique)
		if count != 2 || unique != 2 {
			t.Errorf("history %d %d", count, unique)
		}
		return nil, err
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestPersistenceAndCacheReload(t *testing.T) {
	dbfile := filepath.Join(t.TempDir(), "persist.db")
	p, err := database.Open(dbfile, 2)
	if err != nil {
		t.Fatal(err)
	}
	s, err := database.NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	a := New(s, time.Second, "", static.Handler())
	f := fixture{a, a.Router(), p}
	id := deck(t, f, "")
	ok(t, f, path(id, "draw/?count=2"))
	ok(t, f, path(id, "pile/p/add/?cards=AS"))
	a.Close()
	p.Close()
	p, err = database.Open(dbfile, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s, err = database.NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	a = New(s, time.Second, "", static.Handler())
	defer a.Close()
	f = fixture{a, a.Router(), p}
	r := ok(t, f, path(id, "pile/p/list/"))
	if r.Remaining != 50 || r.Piles["p"].Remaining != 1 {
		t.Fatal(r)
	}
	assertCodes(t, ok(t, f, path(id, "draw/")), "3S")
}
func BenchmarkDrawReturnCycle(b *testing.B) {
	f := setup(b)
	id := deck(b, f, "")
	drawURL := path(id, "draw/")
	returnURL := path(id, "return/")
	b.ResetTimer()
	for b.Loop() {
		status, _, r := request(f.h, drawURL)
		if status != 200 || r.Remaining != 51 {
			b.Fatal(status, r)
		}
		status, _, r = request(f.h, returnURL)
		if status != 200 || r.Remaining != 52 {
			b.Fatal(status, r)
		}
	}
}

func TestConcurrentMultiDeck104(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "?deck_count=2")
	start := make(chan struct{})
	out := make(chan models.Response, 104)
	var wg sync.WaitGroup
	for i := 0; i < 104; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			status, _, r := request(f.h, path(id, "draw/"))
			if status != 200 {
				t.Errorf("multi draw %d", status)
			}
			out <- r
		}()
	}
	close(start)
	wg.Wait()
	close(out)
	counts := map[string]int{}
	remaining := map[int]bool{}
	for r := range out {
		if len(cards(r)) != 1 {
			t.Fatal(r)
		}
		counts[cards(r)[0]]++
		remaining[r.Remaining] = true
	}
	for _, code := range utils.Standard(false) {
		if counts[code] != 2 {
			t.Errorf("%s physical count %d", code, counts[code])
		}
	}
	if len(remaining) != 104 {
		t.Fatal("nonconsecutive remaining", remaining)
	}
	if r := ok(t, f, path(id, "return/")); r.Remaining != 104 {
		t.Fatal("lost physical cards", r)
	}
}
func TestPileCountVariantsAndIsolation(t *testing.T) {
	for _, variant := range []string{"top/", "bottom/", "random/"} {
		t.Run(variant, func(t *testing.T) {
			f := setup(t)
			id := deck(t, f, "")
			ok(t, f, path(id, "draw/?count=7"))
			ok(t, f, path(id, "pile/p/add/?cards=AS,2S,3S,4S,5S"))
			ok(t, f, path(id, "pile/q/add/?cards=6S,7S"))
			r := ok(t, f, path(id, "pile/p/draw/"+variant+"?count=3"))
			if len(cards(r)) != 3 || r.Piles["p"].Remaining != 2 || r.Remaining != 45 {
				t.Fatal(r)
			}
			if variant == "top/" {
				assertCodes(t, r, "5S", "4S", "3S")
			}
			if variant == "bottom/" {
				assertCodes(t, r, "AS", "2S", "3S")
			}
			seen := map[string]bool{}
			for _, code := range cards(r) {
				if seen[code] || !strings.Contains("AS,2S,3S,4S,5S", code) {
					t.Fatal("bad random extraction", cards(r))
				}
				seen[code] = true
			}
			ok(t, f, path(id, "pile/p/shuffle/"))
			other := ok(t, f, path(id, "pile/q/list/"))
			if other.Piles["q"].Remaining != 2 || (*other.Piles["q"].Cards)[0].Code != "6S" {
				t.Fatal("cross-pile contamination", other)
			}
			assertCodes(t, ok(t, f, path(id, "draw/")), "8S")
		})
	}
}
func TestHTTPDatabaseError(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "")
	f.a.history.Wait()
	_, err := f.p.Do(context.Background(), true, func(ctx context.Context, db *sql.DB) (any, error) {
		_, err := db.ExecContext(ctx, "DROP TABLE cards")
		return nil, err
	})
	if err != nil {
		t.Fatal(err)
	}
	errorWant(t, f, path(id, "draw/"), 500, "Database error")
}
func TestHistoryDoesNotDelayResponse(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "")
	f.a.history.Wait()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		f.p.Do(context.Background(), true, func(context.Context, *sql.DB) (any, error) { close(started); <-release; return nil, nil })
	}()
	<-started
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), requestKey{}, "cancelled-parent"))
	cancel()
	// The response has already been written before record is called by finish.
	// A blocked history writer must not block scheduling or inherit cancellation.
	scheduled := make(chan struct{})
	go func() { f.a.record(ctx, "test-history", id); close(scheduled) }()
	select {
	case <-scheduled:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("history scheduling blocked")
	}
	close(release)
	<-done
	f.a.history.Wait()
	_, err := f.p.Do(context.Background(), false, func(ctx context.Context, db *sql.DB) (any, error) {
		var n int
		err := db.QueryRowContext(ctx, "SELECT count(*) FROM historique WHERE request_id='cancelled-parent'").Scan(&n)
		if n != 1 {
			t.Errorf("detached history lost: %d", n)
		}
		return nil, err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAtomicNewDrawFailureAndFastCache(t *testing.T) {
	f := setup(t)
	errorWant(t, f, "/api/deck/new/draw/?count=53", 400, "Not enough cards remaining")
	_, err := f.p.Do(context.Background(), false, func(ctx context.Context, db *sql.DB) (any, error) {
		var n int
		err := db.QueryRowContext(ctx, "SELECT count(*) FROM decks").Scan(&n)
		if n != 0 {
			t.Errorf("failed atomic creation left %d decks", n)
		}
		return nil, err
	})
	if err != nil {
		t.Fatal(err)
	}
	multi := deck(t, f, "?deck_count=2")
	f.a.history.Wait()
	f.p.Close()
	errorWant(t, f, path("unknown", "draw/"), 404, "Deck not found")
	errorWant(t, f, path(multi, "pile/p/list/"), 400, "Pile operations not supported with multiple decks")
}
func TestConcurrentDrawBatches(t *testing.T) {
	f := setup(t)
	id := deck(t, f, "")
	start := make(chan struct{})
	results := make(chan models.Response, 13)
	var wg sync.WaitGroup
	for i := 0; i < 13; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			status, _, r := request(f.h, path(id, "draw/?count=4"))
			if status != 200 {
				t.Errorf("batch draw %d", status)
			}
			results <- r
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	ordered := utils.Standard(false)
	seen := map[int]bool{}
	for r := range results {
		offset := 52 - r.Remaining - 4
		if offset < 0 || offset%4 != 0 || offset > 48 || seen[offset] {
			t.Fatalf("invalid batch offset %d", offset)
		}
		seen[offset] = true
		assertCodes(t, r, ordered[offset:offset+4]...)
	}
	if len(seen) != 13 {
		t.Fatal("missing batch")
	}
}
func BenchmarkDrawOne(b *testing.B) {
	f := setup(b)
	var id string
	i := 0
	for b.Loop() {
		if i%52 == 0 {
			b.StopTimer()
			id = deck(b, f, "")
			f.a.history.Wait()
			b.StartTimer()
		}
		status, _, r := request(f.h, path(id, "draw/"))
		if status != 200 || len(cards(r)) != 1 {
			b.Fatal(status, r)
		}
		i++
	}
}
