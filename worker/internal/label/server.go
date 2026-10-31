package label

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"sync"
)

//go:embed page.html
var page []byte

// Handler serves one session. Writes need a custom header, which a page on another site
// can't send without a preflight that nothing here answers. Job text only ever reaches
// the page as JSON, and the page renders it as text.
func Handler(s *Session) http.Handler {
	var mu sync.Mutex
	mux := http.NewServeMux()
	state := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(s.Current())
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'")
		_, _ = w.Write(page)
	})
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		state(w)
	})
	write := func(do func(string) error) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Label") != "1" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if err := do(r.URL.Query().Get("key")); err != nil {
				http.Error(w, "could not save", http.StatusInternalServerError)
				return
			}
			state(w)
		}
	}
	mux.HandleFunc("POST /press", write(s.Press))
	mux.HandleFunc("POST /undo", write(func(string) error { return s.Undo() }))
	return mux
}
