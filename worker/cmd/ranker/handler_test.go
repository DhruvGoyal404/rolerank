package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DhruvGoyal404/rolerank/worker/internal/embed"
	"github.com/DhruvGoyal404/rolerank/worker/internal/rank"
)

type fakeEmbed struct{ got []string }

func (f *fakeEmbed) Embed(t []string) ([][]float32, error) {
	f.got = t
	return [][]float32{make([]float32, embed.Dims)}, nil
}

type fakeSearch struct {
	p     rank.Profile
	limit int
	err   error
}

func (f *fakeSearch) Search(_ context.Context, p rank.Profile, _ rank.Mode, limit int) ([]rank.Ranked, error) {
	f.p, f.limit = p, limit
	return []rank.Ranked{{Candidate: rank.Candidate{ID: 7}, Score: 0.6, Band: "Strong"}}, f.err
}

const tok = "tttttttttttttttttttttttttttttttttttttttt"

func post(h http.Handler, auth, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestSearchEmbedsWithPrefixAndRanks(t *testing.T) {
	e, s := &fakeEmbed{}, &fakeSearch{}
	w := post(Handler(e, s, tok), "Bearer "+tok, `{"skills":["Go"],"titles":"Backend Engineer","level":"entry","country":"IN","remote":true,"text":"Go developer","limit":500}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var out struct{ Hits []Hit }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Hits) != 1 || out.Hits[0].ID != 7 || out.Hits[0].Band != "Strong" {
		t.Fatalf("hits %+v %v", out, err)
	}
	if e.got[0] != embed.QueryPrefix+"Go developer" {
		t.Fatalf("embedded %q", e.got[0])
	}
	if s.limit != 50 || s.p.Country != "IN" || !s.p.Remote || len(s.p.Titles) == 0 {
		t.Fatalf("profile %+v limit %d", s.p, s.limit)
	}
}

func TestSearchRefusesBadRequests(t *testing.T) {
	h := Handler(&fakeEmbed{}, &fakeSearch{}, tok)
	if w := post(h, "", `{}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := post(h, "Bearer wrong", `{}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", w.Code)
	}
	for _, body := range []string{
		`{"level":"principal","country":"IN"}`,
		`{"level":"entry","country":"India"}`,
		`{"level":"entry","country":"IN","extra":1}`,
		`not json`,
	} {
		if w := post(h, "Bearer "+tok, body); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, w.Code)
		}
	}
}

func TestSearchFailureSaysNothingAboutTheProfile(t *testing.T) {
	w := post(Handler(&fakeEmbed{}, &fakeSearch{err: errors.New("db down: Go developer")}, tok), "Bearer "+tok,
		`{"skills":["Go"],"level":"entry","country":"IN","text":"Go developer"}`)
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "Go developer") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
