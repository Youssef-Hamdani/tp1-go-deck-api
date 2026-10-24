package handlers

import (
	"github.com/Youssef-Hamdani/tp1-go-deck-api/models"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/utils"
	"net/http"
	"strings"
)

func (a *API) pile(w http.ResponseWriter, r *http.Request) {
	id, name := r.PathValue("deck_id"), r.PathValue("pile_name")
	if err := a.known(id, true); err != nil {
		fail(w, id, err)
		return
	}
	if len(name) > 100 || strings.TrimSpace(name) == "" {
		fail(w, id, models.Fail("Invalid parameter", 400))
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	op := parts[5]
	variant := "top"
	if len(parts) > 6 {
		variant = parts[6]
	}
	q := r.URL.Query()
	var codes []string
	var err error
	n := 1
	if q.Has("cards") && (op == "add" || op == "draw" || op == "return") {
		codes, err = utils.Codes(q.Get("cards"))
	} else if op == "add" {
		err = models.Fail("Invalid parameter", 400)
	}
	if err == nil && op == "draw" {
		if codes != nil && q.Has("count") {
			err = models.Fail("Invalid parameter", 400)
		} else {
			n, err = utils.Integer(q, "count", 1, 1080)
		}
	}
	if err != nil {
		fail(w, id, err)
		return
	}
	base := a.base(r)
	v, err := a.Store.Operate(r.Context(), id, op != "list", nil, func(d *models.Deck) (any, error) {
		if op != "add" && !d.Piles[name] {
			return nil, models.Fail("Pile not found", 404)
		}
		switch op {
		case "add":
			chosen, err := selectCodes(d, indices(d, "drawn", ""), codes, false)
			if err != nil {
				return nil, err
			}
			d.Piles[name] = true
			move(d, chosen, "pile", name)
		case "list":
			return response(d, base, false, true, name, nil), nil
		case "shuffle":
			shuffle(d, indices(d, "pile", name))
		case "draw":
			ids := indices(d, "pile", name)
			var chosen []int
			var err error
			if codes != nil {
				chosen, err = selectCodes(d, ids, codes, true)
			} else {
				chosen, err = draw(d, ids, n, true, variant == "top", variant == "random")
			}
			if err != nil {
				return nil, err
			}
			move(d, chosen, "drawn", "")
			return response(d, base, false, true, "", chosen), nil
		case "return":
			ids := indices(d, "pile", name)
			if codes != nil {
				var err error
				ids, err = selectCodes(d, ids, codes, true)
				if err != nil {
					return nil, err
				}
			}
			move(d, ids, "deck", "")
			return response(d, base, true, true, "", nil), nil
		default:
			return nil, models.Fail("Invalid parameter", 400)
		}
		return response(d, base, false, true, "", nil), nil
	})
	historyOp := "pile/" + op
	if op == "list" {
		historyOp = ""
	}
	a.finish(w, r, v, err, historyOp)
}
