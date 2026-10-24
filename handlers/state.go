package handlers

import (
	"github.com/Youssef-Hamdani/tp1-go-deck-api/models"
	"github.com/Youssef-Hamdani/tp1-go-deck-api/utils"
	"math/rand/v2"
	"sort"
)

func indices(d *models.Deck, location, pile string) []int {
	out := []int{}
	for i, c := range d.Cards {
		if c.Location == location && c.Pile == pile {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := d.Cards[out[i]], d.Cards[out[j]]
		if a.Position == b.Position {
			return a.ID < b.ID
		}
		return a.Position < b.Position
	})
	return out
}
func shuffle(d *models.Deck, ids []int) {
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	for pos, i := range ids {
		d.Cards[i].Position = pos
	}
}
func move(d *models.Deck, ids []int, location, pile string) {
	pos := -1
	for _, c := range d.Cards {
		if c.Location == location && c.Pile == pile && c.Position > pos {
			pos = c.Position
		}
	}
	for _, i := range ids {
		pos++
		d.Cards[i].Location = location
		d.Cards[i].Pile = pile
		d.Cards[i].Position = pos
	}
}
func selectCodes(d *models.Deck, ids []int, codes []string, pile bool) ([]int, error) {
	out := []int{}
	used := map[int]bool{}
	for _, code := range codes {
		found := -1
		for _, i := range ids {
			if !used[i] && d.Cards[i].Code == code {
				found = i
				break
			}
		}
		if found < 0 {
			msg := "Card not found in deck"
			if pile {
				msg = "Card not found in pile"
			}
			return nil, models.Fail(msg, 404)
		}
		used[found] = true
		out = append(out, found)
	}
	return out, nil
}
func draw(d *models.Deck, ids []int, n int, pile, top, random bool) ([]int, error) {
	if len(ids) == 0 {
		msg := "Deck is empty"
		if pile {
			msg = "Pile is empty"
		}
		return nil, models.Fail(msg, 400)
	}
	if n > len(ids) {
		return nil, models.Fail("Not enough cards remaining", 400)
	}
	if random {
		rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	} else if top {
		for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
			ids[i], ids[j] = ids[j], ids[i]
		}
	}
	return ids[:n], nil
}
func response(d *models.Deck, base string, withShuffle, withPiles bool, list string, drawn []int) models.Response {
	r := models.Response{Success: true, DeckID: d.ID, Remaining: len(indices(d, "deck", ""))}
	if withShuffle {
		b := d.Shuffled
		r.Shuffled = &b
	}
	if withPiles {
		r.Piles = map[string]models.Pile{}
		for name := range d.Piles {
			ids := indices(d, "pile", name)
			p := models.Pile{Remaining: len(ids)}
			if name == list {
				cards := []models.Card{}
				for _, i := range ids {
					cards = append(cards, utils.Card(d.Cards[i].Code, base))
				}
				p.Cards = &cards
			}
			r.Piles[name] = p
		}
	}
	if drawn != nil {
		cards := []models.Card{}
		for _, i := range drawn {
			cards = append(cards, utils.Card(d.Cards[i].Code, base))
		}
		r.Cards = &cards
	}
	return r
}
