package handlers

import (
	"github.com/Youssef-Hamdani/tp1-go-deck-api/models"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/utils"
	"net/http"
	"strings"
)

func (a *API) create(w http.ResponseWriter, r *http.Request) { a.createDeck(w, r, false, false) }
func (a *API) createDeck(w http.ResponseWriter, r *http.Request, shuffled, drawNew bool) {
	q := r.URL.Query()
	count, err := utils.Integer(q, "deck_count", 1, 20)
	if err != nil {
		fail(w, "new", err)
		return
	}
	jokers, err := utils.Boolean(q, "jokers_enabled")
	if err != nil {
		fail(w, "new", err)
		return
	}
	codes := utils.Standard(jokers)
	if q.Has("cards") {
		codes, err = utils.Codes(q.Get("cards"))
		if err != nil {
			fail(w, "new", err)
			return
		}
	}
	n := 1
	if drawNew {
		n, err = utils.Integer(q, "count", 1, 1080)
		if err != nil {
			fail(w, "new", err)
			return
		}
	}
	d := &models.Deck{Count: count, Shuffled: shuffled, Piles: map[string]bool{}}
	for k := 0; k < count; k++ {
		for _, code := range codes {
			i := len(d.Cards)
			d.Cards = append(d.Cards, models.PhysicalCard{ID: i, Code: code, Location: "deck", Position: i})
		}
	}
	base := a.base(r)
	v, err := a.Store.Operate(r.Context(), "", true, d, func(d *models.Deck) (any, error) {
		ids := indices(d, "deck", "")
		if shuffled {
			shuffle(d, ids)
		}
		if drawNew {
			ids = indices(d, "deck", "")
			chosen, err := draw(d, ids, n, false, false, false)
			if err != nil {
				return nil, err
			}
			move(d, chosen, "drawn", "")
			return response(d, base, false, false, "", chosen), nil
		}
		return response(d, base, true, false, "", nil), nil
	})
	op := "create"
	if drawNew {
		op = "create+draw"
	}
	a.finish(w, r, v, err, op)
}
func (a *API) deck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("deck_id")
	op := strings.TrimSuffix(r.URL.Path, "/")
	op = op[strings.LastIndex(op, "/")+1:]
	if id == "new" && (op == "shuffle" || op == "draw") {
		a.createDeck(w, r, true, op == "draw")
		return
	}
	if err := a.known(id, false); err != nil {
		fail(w, id, err)
		return
	}
	q := r.URL.Query()
	n := 1
	var err error
	remaining := false
	var codes []string
	switch op {
	case "draw":
		n, err = utils.Integer(q, "count", 1, 1080)
	case "shuffle":
		remaining, err = utils.Boolean(q, "remaining")
	case "return":
		if q.Has("cards") {
			codes, err = utils.Codes(q.Get("cards"))
		}
	}
	if err != nil {
		fail(w, id, err)
		return
	}
	base := a.base(r)
	v, err := a.Store.Operate(r.Context(), id, true, nil, func(d *models.Deck) (any, error) {
		switch op {
		case "draw":
			chosen, err := draw(d, indices(d, "deck", ""), n, false, false, false)
			if err != nil {
				return nil, err
			}
			move(d, chosen, "drawn", "")
			return response(d, base, false, false, "", chosen), nil
		case "shuffle":
			if !remaining {
				for i := range d.Cards {
					d.Cards[i].Location = "deck"
					d.Cards[i].Pile = ""
					d.Cards[i].Position = d.Cards[i].ID
				}
			}
			shuffle(d, indices(d, "deck", ""))
			d.Shuffled = true
			return response(d, base, true, false, "", nil), nil
		case "return":
			ids := indices(d, "drawn", "")
			if codes != nil {
				var err error
				ids, err = selectCodes(d, ids, codes, false)
				if err != nil {
					return nil, err
				}
			}
			move(d, ids, "deck", "")
			return response(d, base, true, true, "", nil), nil
		}
		return nil, models.Fail("Invalid parameter", 400)
	})
	a.finish(w, r, v, err, op)
}
