package rank

import (
	"context"
	"fmt"
	"slices"
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

// Hard filters, never violated: the level window and geography. A job is open to the profile if
// it lists the profile's country, or, when the profile accepts remote work, it is remote for
// anyone.
const eligible = `level = ANY($1) AND ($2 = ANY(countries) OR ($3 AND 'remote_global' = ANY(remote_scopes)))`

// Searcher runs searches against db-corpus.
type Searcher struct {
	Pool    *pgxpool.Pool
	Weights Weights
	Bands   Bands
	Now     func() time.Time
}

// Search returns up to limit ranked jobs.
func (s *Searcher) Search(ctx context.Context, p Profile, mode Mode, limit int) ([]Ranked, error) {
	args := []any{Levels(p.Level), p.Country, p.Remote}
	var vec, lex []int64
	var err error
	if mode != Lexical {
		if len(p.Embedding) != embed.Dims {
			return nil, fmt.Errorf("profile embedding has %d dimensions", len(p.Embedding))
		}
		// "+ 0" keeps the planner off the HNSW index: exact search over the filtered subset,
		// recall 1.0, the normal case for selective filters (ARCHITECTURE.md, Phase 0 spike).
		vec, err = s.ids(ctx, `SELECT posting_id FROM search_postings WHERE embedding IS NOT NULL AND `+eligible+`
			ORDER BY (embedding <=> $4::halfvec) + 0, posting_id LIMIT $5`, append(args, embed.Literal(p.Embedding), PerRetriever)...)
		if err != nil {
			return nil, err
		}
	}
	if mode != Vector {
		lex, err = s.ids(ctx, `SELECT posting_id FROM search_postings WHERE `+eligible+`
			AND (skills_required && $4 OR skills_nice && $4 OR title_terms && $5)
			ORDER BY 2 * cardinality(ARRAY(SELECT unnest(skills_required) INTERSECT SELECT unnest($4::text[])))
			  + cardinality(ARRAY(SELECT unnest(skills_nice) INTERSECT SELECT unnest($4::text[])))
			  + cardinality(ARRAY(SELECT unnest(title_terms) INTERSECT SELECT unnest($5::text[]))) DESC, posting_id
			LIMIT $6`, append(args, p.Skills, p.Titles, PerRetriever)...)
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
