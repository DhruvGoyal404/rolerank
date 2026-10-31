package useragent

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	got := String("0.1.0")
	want := "RoleRankBot/0.1.0 (+https://rolerank.dhruvgoyal.tech/about/bot; bot@dhruvgoyal.tech)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNoPersonalAddress(t *testing.T) {
	if strings.Contains(String("x"), "gmail") {
		t.Fatal("the User-Agent must not carry a personal address")
	}
}
