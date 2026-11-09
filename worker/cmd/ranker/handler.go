// Command ranker serves job search to the API on the internal network (P6): POST /search with a
// profile's skills, title terms, level, place and text returns ranked postings. Every request
// needs the shared token in Authorization. The profile text is embedded here, with bge's query
// prefix as P3 chose, and the hybrid search of P3 runs as corpus_ro. Profile content is never
// logged.
package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/DhruvGoyal404/rolerank/worker/internal/embed"
	"github.com/DhruvGoyal404/rolerank/worker/internal/rank"
	"github.com/DhruvGoyal404/rolerank/worker/internal/skills"
)

type embedder interface {
	Embed(texts []string) ([][]float32, error)
}

type searcher interface {
	Search(ctx context.Context, p rank.Profile, mode rank.Mode, limit int) ([]rank.Ranked, error)
	ScoreOne(ctx context.Context, p rank.Profile, id int64) (*rank.Ranked, error)
}

// Query is what the API sends. Skills are canonical taxonomy names; titles are the user's target
// titles as written, turned into title terms here.
type Query struct {
	Skills  []string `json:"skills"`
	Titles  string   `json:"titles"`
	Level   string   `json:"level"`
	Country string   `json:"country"`
	Remote  bool     `json:"remote"`
	Text    string   `json:"text"`
	Limit   int      `json:"limit"`
	ID      int64    `json:"id,omitempty"` // POST /score only: the posting to score
}

// Hit is one ranked posting: the explainable features, the score and its fit band.
type Hit struct {
	ID       int64         `json:"id"`
	Score    float64       `json:"score"`
	Band     string        `json:"band"`
	Features rank.Features `json:"features"`
}

var levels = map[string]bool{"intern": true, "entry": true, "mid": true, "senior": true}
var iso = regexp.MustCompile(`^[A-Z]{2}$`)

const maxBody = 64 << 10

// Handler serves POST /search, POST /score and GET /health.
func Handler(e embedder, s searcher, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	// read authenticates, validates and embeds the query; it writes the error itself.
	read := func(w http.ResponseWriter, req *http.Request) (Query, rank.Profile, bool) {
		var q Query
		if subtle.ConstantTimeCompare([]byte(req.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return q, rank.Profile{}, false
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxBody))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&q); err != nil || !levels[q.Level] || !iso.MatchString(q.Country) || len(q.Skills) > 200 || len(q.Text) > 4000 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return q, rank.Profile{}, false
		}
		vec, err := e.Embed([]string{embed.QueryPrefix + q.Text})
		if err != nil || len(vec) != 1 {
			slog.Warn("embed failed")
			http.Error(w, "search failed", http.StatusBadGateway)
			return q, rank.Profile{}, false
		}
		return q, rank.Profile{Skills: q.Skills, Titles: skills.TitleTerms(q.Titles), Level: q.Level, Country: q.Country, Remote: q.Remote, Embedding: vec[0]}, true
	}
	reply := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /search", func(w http.ResponseWriter, req *http.Request) {
		q, p, ok := read(w, req)
		if !ok {
			return
		}
		got, err := s.Search(req.Context(), p, rank.Hybrid, max(1, min(q.Limit, 50)))
		if err != nil {
			slog.Warn("search failed")
			http.Error(w, "search failed", http.StatusBadGateway)
			return
		}
		hits := make([]Hit, 0, len(got))
		for _, r := range got {
			hits = append(hits, Hit{ID: r.ID, Score: r.Score, Band: r.Band, Features: r.Features})
		}
		reply(w, map[string]any{"hits": hits})
	})
	// One posting's fit for the profile, as a search would rank it (rank.Searcher.ScoreOne).
	mux.HandleFunc("POST /score", func(w http.ResponseWriter, req *http.Request) {
		q, p, ok := read(w, req)
		if !ok {
			return
		}
		if q.ID <= 0 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		r, err := s.ScoreOne(req.Context(), p, q.ID)
		if err != nil {
			slog.Warn("score failed")
			http.Error(w, "search failed", http.StatusBadGateway)
			return
		}
		if r == nil {
			reply(w, map[string]any{"hit": nil})
			return
		}
		reply(w, map[string]any{"hit": Hit{ID: r.ID, Score: r.Score, Band: r.Band, Features: r.Features}})
	})
	return mux
}
