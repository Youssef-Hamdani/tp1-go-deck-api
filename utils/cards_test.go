package utils

import (
	"reflect"
	"testing"
)

func TestCodes(t *testing.T) {
	got, err := Codes("as,0h, KD")
	if err != nil || !reflect.DeepEqual(got, []string{"AS", "10H", "KD"}) {
		t.Fatalf("%v %v", got, err)
	}
	for _, input := range []string{"AS,as", "0H,10H", "99Z", "AS,", "", "1S"} {
		if _, err := Codes(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}
func TestStandardAndFrench(t *testing.T) {
	codes := Standard(true)
	if len(codes) != 54 || codes[0] != "AS" || codes[13] != "AD" || codes[26] != "AC" || codes[39] != "AH" || codes[53] != "X2" {
		t.Fatal(codes)
	}
	for _, code := range codes {
		if _, err := Normalize(code); err != nil {
			t.Fatal(err)
		}
	}
	c := Card("QH", "http://localhost:8080")
	if c.Value != "REINE" || c.Suit != "COEUR" || c.Image != "http://localhost:8080/static/img/QH.svg" {
		t.Fatal(c)
	}
}
