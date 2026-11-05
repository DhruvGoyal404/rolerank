package rank

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DhruvGoyal404/rolerank/worker/internal/embed"
)

// Mode picks the pipeline. Lexical and Vector are the baselines the hybrid must beat (EVALS.md).
type Mode string

const (
	Lexical Mode = "lexical"
	Vector  Mode = "vector"
	Hybrid  Mode = "hybrid"
)

// PerRetriever is how many candidates each retriever passes on.
const PerRetriever = 200

// Eligible is the hard filter, never violated: the level window ($1) and geography. A job is
// open to the profile if it lists the profile's country, or, when the profile accepts remote
// work, it is remote for anyone. The country is written into the SQL, not passed as a
// parameter, so the planner can match a per-country partial index (migration 9); it is checked
// to be two capital letters first.
func Eligible(p Profile) (string, error) {
	if !country.MatchString(p.Country) {
		return "", fmt.Errorf("country %q isn't an ISO code", p.Country)
	}
	geo := "'" + p.Country + "' = ANY(countries)"
	if p.Remote {
		geo = "(" + geo + " OR 'remote_global' = ANY(remote_scopes))"
	}
	return "level = ANY($1) AND " + geo, nil
}

var country = regexp.MustCompile(`^[A-Z]{2}$`)

// LargeCorpus is the row count above which search switches to its large-corpus plan. Below it
// (the real corpus: 21,279 rows on 2026-11-05) vectors are searched exactly and the lexical
// retriever runs. Above it, an exact scan and the lexical scan both cost seconds (bench, 5 lakh
// rows, 2026-11-05), so vectors go through HNSW with iterative scan and the lexical retriever is
// skipped; skill coverage still counts in the rerank.
const LargeCorpus = 50000

// Searcher runs searches against db-corpus.
type Searcher struct {
	Pool    *pgxpool.Pool
	Weights Weights
	Bands   Bands
	Now     func() time.Time

	mu     sync.Mutex
	sizeAt time.Time
	large  bool
}

// isLarge reads the planner's row estimate, at most once a minute.
func (s *Searcher) isLarge(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.sizeAt) < time.Minute {
		return s.large, nil
	}
	var rows float64
	if err := s.Pool.QueryRow(ctx, `SELECT reltuples FROM pg_class WHERE oid = 'search_postings'::regclass`).Scan(&rows); err != nil {
		return false, fmt.Errorf("corpus size: %w", err)
	}
	s.large, s.sizeAt = rows > LargeCorpus, time.Now()
	return s.large, nil
}

// Search returns up to limit ranked jobs.
func (s *Searcher) Search(ctx context.Context, p Profile, mode Mode, limit int) ([]Ranked, error) {
	args := []any{Levels(p.Level)}
	where, err := Eligible(p)
	if err != nil {
		return nil, err
	}
	large, err := s.isLarge(ctx)
	if err != nil {
		return nil, err
	}
	var vec, lex []int64
	if mode != Lexical {
		if len(p.Embedding) != embed.Dims {
			return nil, fmt.Errorf("profile embedding has %d dimensions", len(p.Embedding))
		}
		if vec, err = s.vectorIDs(ctx, large, where, append(args, embed.Literal(p.Embedding), PerRetriever)); err != nil {
			return nil, err
		}
	}
	if mode == Lexical || (mode == Hybrid && !large) {
		lex, err = s.ids(ctx, `SELECT posting_id FROM search_postings WHERE `+where+`
			AND (skills_required && $2 OR skills_nice && $2 OR title_terms && $3)
			ORDER BY 2 * (SELECT count(*) FROM unnest(skills_required) x WHERE x = ANY($2))
			  + (SELECT count(*) FROM unnest(skills_nice) x WHERE x = ANY($2))
			  + (SELECT count(*) FROM unnest(title_terms) x WHERE x = ANY($3)) DESC, posting_id
			LIMIT $4`, append(args, p.Skills, p.Titles, PerRetriever)...)
		if err != nil {
			return nil, err
		}
	}
	cands, err := s.candidates(ctx, p, vec, lex)
	if err != nil {
		return nil, err
	}
	var out []Ranked
	switch mode {
	case Hybrid:
		out = Rerank(p, cands, s.Weights, s.Bands, s.Now())
	default: // a baseline keeps its retriever's own order
		order := vec
		if mode == Lexical {
			order = lex
		}
		byID := map[int64]Candidate{}
		for _, c := range cands {
			byID[c.ID] = c
		}
		for _, id := range order {
			c := byID[id]
			f := Score(p, c, s.Now())
			out = append(out, Ranked{Candidate: c, Features: f, Score: s.Weights.Total(f)})
		}
	}
	return out[:min(limit, len(out))], nil
}

// vectorIDs returns the nearest eligible jobs, nearest first.
func (s *Searcher) vectorIDs(ctx context.Context, large bool, where string, args []any) ([]int64, error) {
	if !large {
		// "+ 0" keeps the planner off the HNSW index: exact search over the filtered subset,
		// recall 1.0, the normal case for selective filters (ARCHITECTURE.md, Phase 0 spike).
		return s.ids(ctx, `SELECT posting_id FROM search_postings WHERE embedding IS NOT NULL AND `+where+`
			ORDER BY (embedding <=> $2::halfvec) + 0, posting_id LIMIT $3`, args...)
	}
	// HNSW with iterative scan keeps walking the graph until enough rows pass the filters.
	// relaxed_order can return rows slightly out of order, so they're sorted again by distance.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL hnsw.iterative_scan = relaxed_order; SET LOCAL hnsw.ef_search = `+fmt.Sprint(EfSearch)); err != nil {
		return nil, fmt.Errorf("hnsw settings: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT posting_id FROM (SELECT posting_id, embedding <=> $2::halfvec AS d FROM search_postings
		WHERE `+where+` ORDER BY embedding <=> $2::halfvec LIMIT $3) n ORDER BY d, posting_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// EfSearch is HNSW's search breadth on the large-corpus plan; the bench measures its recall
// against exact search.
var EfSearch = 200

func (s *Searcher) ids(ctx context.Context, sql string, args ...any) ([]int64, error) {
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

func (s *Searcher) candidates(ctx context.Context, p Profile, vec, lex []int64) ([]Candidate, error) {
	ids := append(slices.Clone(vec), lex...)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	vector := "[" + "0" + "]"
	cosine := "0::float8"
	args := []any{ids, p.Country}
	if len(p.Embedding) == embed.Dims {
		vector = embed.Literal(p.Embedding)
		cosine = "COALESCE(1 - (embedding <=> $3::halfvec), 0)"
		args = append(args, vector)
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT posting_id, level, skills_required, skills_nice, title_terms, $2 = ANY(countries),
		  COALESCE(posted_at, 'epoch'), `+cosine+`
		FROM search_postings WHERE posting_id = ANY($1)`, args...)
	if err != nil {
		return nil, fmt.Errorf("candidates: %w", err)
	}
	rank := func(list []int64) map[int64]int {
		m := map[int64]int{}
		for i, id := range list {
			m[id] = i + 1
		}
		return m
	}
	vr, lr := rank(vec), rank(lex)
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Candidate, error) {
		var c Candidate
		err := r.Scan(&c.ID, &c.Level, &c.Required, &c.Nice, &c.Title, &c.Onsite, &c.Posted, &c.Cosine)
		if c.Posted.Equal(time.Unix(0, 0)) {
			c.Posted = time.Time{}
		}
		c.VecRank, c.LexRank = vr[c.ID], lr[c.ID]
		return c, err
	})
}
