// Package utils implements card codes and strict query validation.
package utils

import (
	"github.com/Youssef-Hamdani/tp1-go-deck-api/models"
	"net/url"
	"strconv"
	"strings"
)

func Normalize(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) == 2 && code[0] == '0' {
		code = "10" + code[1:]
	}
	if code == "X1" || code == "X2" {
		return code, nil
	}
	if len(code) < 2 || len(code) > 3 {
		return "", models.Fail("Invalid card code", 400)
	}
	suit := code[len(code)-1:]
	rank := code[:len(code)-1]
	if !strings.Contains("SDCH", suit) {
		return "", models.Fail("Invalid card code", 400)
	}
	switch rank {
	case "A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K":
		return code, nil
	}
	return "", models.Fail("Invalid card code", 400)
}
func Codes(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, models.Fail("Invalid parameter", 400)
	}
	out := []string{}
	seen := map[string]bool{}
	for _, v := range strings.Split(raw, ",") {
		c, err := Normalize(v)
		if err != nil {
			return nil, err
		}
		if seen[c] {
			return nil, models.Fail("Duplicate cards in request", 400)
		}
		seen[c] = true
		out = append(out, c)
	}
	return out, nil
}
func Standard(jokers bool) []string {
	out := []string{}
	for _, s := range []string{"S", "D", "C", "H"} {
		for _, v := range []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"} {
			out = append(out, v+s)
		}
	}
	if jokers {
		out = append(out, "X1", "X2")
	}
	return out
}
func Card(code, base string) models.Card {
	c := models.Card{Code: code, Image: base + "/static/img/" + code + ".svg"}
	if code == "X1" || code == "X2" {
		c.Value = "JOKER"
		c.Suit = "NOIR"
		if code == "X2" {
			c.Suit = "ROUGE"
		}
		return c
	}
	c.Value = code[:len(code)-1]
	switch c.Value {
	case "A":
		c.Value = "AS"
	case "J":
		c.Value = "VALET"
	case "Q":
		c.Value = "REINE"
	case "K":
		c.Value = "ROI"
	}
	c.Suit = map[byte]string{'S': "PIQUE", 'D': "CARREAU", 'C': "TREFLE", 'H': "COEUR"}[code[len(code)-1]]
	return c
}
func Integer(q url.Values, key string, def, max int) (int, error) {
	if !q.Has(key) {
		return def, nil
	}
	if len(q[key]) != 1 {
		return 0, models.Fail("Invalid parameter", 400)
	}
	n, err := strconv.Atoi(q.Get(key))
	if err != nil {
		return 0, models.Fail("Invalid parameter", 400)
	}
	if n < 1 || n > max {
		return 0, models.Fail("Parameter out of range", 400)
	}
	return n, nil
}
func Boolean(q url.Values, key string) (bool, error) {
	if !q.Has(key) {
		return false, nil
	}
	if len(q[key]) != 1 {
		return false, models.Fail("Invalid parameter", 400)
	}
	switch q.Get(key) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, models.Fail("Invalid parameter", 400)
}
