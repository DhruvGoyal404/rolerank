package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DhruvGoyal404/rolerank/worker/internal/extract"
)

type fake struct{ calls int }

func (f *fake) Run(_ context.Context, b []byte) (extract.Result, error) {
	f.calls++
	if bytes.HasPrefix(b, []byte("bad")) {
		return extract.Result{}, fmt.Errorf("%w: nope", extract.ErrRejected)
	}
	return extract.Result{Kind: "pdf", Lines: []extract.Line{{N: 1, Text: "Jane"}}}, nil
}

// A dummy, built at run time so no secret-shaped literal sits in the source.
var token = strings.Repeat("t", 32)

func post(h http.Handler, auth string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/extract", bytes.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestExtractNeedsTheToken(t *testing.T) {
	f := &fake{}
	h := Handler(f, token, 2)
	for _, auth := range []string{"", "Bearer wrong", token} {
		if w := post(h, auth, []byte("%PDF-")); w.Code != http.StatusUnauthorized {
			t.Errorf("auth %q: %d", auth, w.Code)
		}
	}
	if f.calls != 0 {
		t.Fatal("ran without the token")
	}
}

func TestExtract(t *testing.T) {
	f := &fake{}
	h := Handler(f, token, 2)
	w := post(h, "Bearer "+token, []byte("%PDF-x"))
	var res extract.Result
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &res) != nil || res.Lines[0].Text != "Jane" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := post(h, "Bearer "+token, []byte("bad")); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("rejected file: %d", w.Code)
	}
	if w := post(h, "Bearer "+token, make([]byte, extract.MaxBytes+1)); w.Code != http.StatusRequestEntityTooLarge || f.calls != 2 {
		t.Fatalf("oversized: %d, calls %d", w.Code, f.calls)
	}
}
