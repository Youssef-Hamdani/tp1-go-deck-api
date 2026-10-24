// Package models contains persistent state and the public response contract.
package models

type Card struct {
	Code  string `json:"code"`
	Image string `json:"image"`
	Value string `json:"value"`
	Suit  string `json:"suit"`
}
type PhysicalCard struct {
	ID                   int
	Code, Location, Pile string
	Position             int
}
type Deck struct {
	ID       string
	Count    int
	Shuffled bool
	Cards    []PhysicalCard
	Piles    map[string]bool
}
type Pile struct {
	Remaining int     `json:"remaining"`
	Cards     *[]Card `json:"cards,omitempty"`
}
type Response struct {
	Success   bool            `json:"success"`
	DeckID    string          `json:"deck_id"`
	Shuffled  *bool           `json:"shuffled,omitempty"`
	Remaining int             `json:"remaining"`
	Cards     *[]Card         `json:"cards,omitempty"`
	Piles     map[string]Pile `json:"piles,omitempty"`
}
type APIError struct {
	Message string
	Status  int
}

func (e *APIError) Error() string           { return e.Message }
func Fail(message string, status int) error { return &APIError{message, status} }
