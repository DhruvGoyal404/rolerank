package label

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DhruvGoyal404/rolerank/worker/internal/dedupe"
)

const postingColumns = `p.id, s.name, p.source_job_id, p.company, p.title, COALESCE(p.location_raw, ''),
	left(p.body, 2500), p.url, p.level, p.is_engineering, p.remote_scope, 'IN' = ANY(p.countries)`

// D6Strata sets how the 300 postings are drawn. About 60% of each group is a plain random
// draw, which gives unbiased scores. The rest targets cases likely to be wrong, which is
// where labels teach the most, and it's scored separately so it can't flatter or sink the
// headline number.
var D6Strata = []struct {
	Name  string
	Count int
	Where string
}{
	{"india-random", 60, `'IN' = ANY(p.countries)`},
	{"india-hard", 40, `'IN' = ANY(p.countries) AND (p.level = 'unknown' OR p.remote_scope = 'unknown'
		OR p.title ~* '(solutions|support|field|customer|sales|implementation|test|analyst)')`},
	{"early-random", 60, `p.is_engineering AND p.level IN ('intern', 'entry')`},
	{"early-hard", 40, `p.is_engineering AND p.level IN ('unknown', 'mid')
		AND (p.body ~* '(fresher|new grad|recent graduate|0[- ]?(to|-)[- ]?[12] years|entry[- ]level)'
		  OR p.title ~* '(associate|graduate|junior|trainee)')`},
	{"random", 70, `true`},
	{"remote-hard", 30, `p.remote_scope IN ('unknown', 'remote_global')`},
}

func sideFrom(row pgx.CollectableRow) (int64, Side, map[string]string, error) {
	var id int64
	var side Side
	var level, scope string
	var engineering, india bool
	err := row.Scan(&id, &side.Source, &side.ID, &side.Company, &side.Title, &side.Location,
		&side.Body, &side.URL, &level, &engineering, &scope, &india)
	side.ID = side.Source + ":" + side.ID
	return id, side, map[string]string{
		"level": level, "engineering": fmt.Sprint(engineering), "remote_scope": scope, "india": fmt.Sprint(india),
	}, err
}

// SelectD6 draws the classifier set. The seed makes a draw repeatable.
func SelectD6(ctx context.Context, pool *pgxpool.Pool, seed string) ([]Item, map[string]int, error) {
	seen := map[int64]bool{}
	var items []Item
	counts := map[string]int{}
	for _, stratum := range D6Strata {
		// Overdraw, because a posting can belong to two strata.
		rows, err := pool.Query(ctx, `SELECT `+postingColumns+`
			FROM postings p JOIN sources s ON s.id = p.source_id
			WHERE p.closed_at IS NULL AND length(p.body) > 200 AND `+stratum.Where+`
			ORDER BY md5($1 || p.id::text) LIMIT $2`, seed+stratum.Name, stratum.Count*4)
		if err != nil {
			return nil, nil, fmt.Errorf("select %s: %w", stratum.Name, err)
		}
		type picked struct {
			id     int64
			side   Side
			system map[string]string
		}
		list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (picked, error) {
			id, side, system, err := sideFrom(row)
			return picked{id, side, system}, err
		})
		if err != nil {
			return nil, nil, fmt.Errorf("select %s: %w", stratum.Name, err)
		}
		for _, p := range list {
			if counts[stratum.Name] == stratum.Count {
				break
			}
			if seen[p.id] {
				continue
			}
			seen[p.id] = true
			counts[stratum.Name]++
			items = append(items, Item{ID: p.side.ID, Stratum: stratum.Name, A: p.side, System: p.system})
		}
	}
	// Mixed, so the hard ones aren't all together and don't tire or anchor the labeller.
	mix(items, seed)
	return items, counts, nil
}

func mix(items []Item, seed string) {
	var a [32]byte
	copy(a[:], seed)
	random := rand.New(rand.NewChaCha8(a))
	random.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
}

type candidate struct {
	id        int64
	side      Side
	cluster   int64
	key       string
	location  string
	signature []byte
}

// D5Bands sets how the 300 pairs are drawn, by what they test.
var D5Bands = []struct {
	Name  string
	Count int
}{
	{"merged", 80},         // precision: pairs the system merged
	{"near-threshold", 90}, // the boundary, where the threshold is decided
	{"other-city", 40},     // near-identical text in two cities: the rule says different
	{"reworded-title", 60}, // recall: same employer and text, but a different key
	{"cross-source", 20},   // an aggregator against an ATS
	{"easy-negative", 10},  // sanity: same title, different text
}

const perCompanyCap = 150

// SelectD5 draws the dedupe pairs from in-scope postings.
func SelectD5(ctx context.Context, pool *pgxpool.Pool, seed string) ([]Item, map[string]int, error) {
	rows, err := pool.Query(ctx, `SELECT `+postingColumns+`, COALESCE(p.cluster_id, 0)
		FROM postings p JOIN sources s ON s.id = p.source_id
		WHERE p.closed_at IS NULL AND p.is_engineering AND length(p.body) > 300`)
	if err != nil {
		return nil, nil, fmt.Errorf("select d5: %w", err)
	}
	defer rows.Close()
	companies := map[string][]candidate{}
	for rows.Next() {
		var c candidate
		var level, scope, source, jobID string
		var engineering, india bool
		err := rows.Scan(&c.id, &source, &jobID, &c.side.Company, &c.side.Title, &c.side.Location,
			&c.side.Body, &c.side.URL, &level, &engineering, &scope, &india, &c.cluster)
		if err != nil {
			return nil, nil, err
		}
		c.side.Source, c.side.ID = source, source+":"+jobID
		c.key = dedupe.Key(c.side.Company, c.side.Title)
		c.location = dedupe.Location(c.side.Location)
		company, _, _ := cutKey(c.key)
		companies[company] = append(companies[company], c)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	bands := map[string][]Item{}
	add := func(band string, a, b candidate, similarity float64) {
		right := b.side
		bands[band] = append(bands[band], Item{
			ID: a.side.ID + "|" + b.side.ID, Stratum: band, A: a.side, B: &right,
			System: map[string]string{
				"merged": fmt.Sprint(a.cluster != 0 && a.cluster == b.cluster), "similarity": fmt.Sprintf("%.3f", similarity),
			},
		})
	}
	names := make([]string, 0, len(companies))
	for name := range companies {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		group := companies[name]
		if len(group) < 2 {
			continue
		}
		if len(group) > perCompanyCap {
			sort.Slice(group, func(i, j int) bool { return group[i].id < group[j].id })
			group = group[:perCompanyCap]
		}
		for i := range group {
			group[i].signature = dedupe.Signature(group[i].side.Body)
		}
		for i := range group {
			for j := i + 1; j < len(group); j++ {
				a, b := group[i], group[j]
				similarity := dedupe.Similarity(a.signature, b.signature)
				aggregator := dedupe.IsAggregator(a.side.Source) != dedupe.IsAggregator(b.side.Source)
				sameKey := a.key == b.key
				switch {
				case aggregator && similarity >= 0.5:
					add("cross-source", a, b, similarity)
				case sameKey && a.cluster != 0 && a.cluster == b.cluster:
					add("merged", a, b, similarity)
				case sameKey && a.location != b.location && similarity >= 0.8:
					add("other-city", a, b, similarity)
				case sameKey && similarity >= 0.5:
					add("near-threshold", a, b, similarity)
				case !sameKey && similarity >= 0.6:
					add("reworded-title", a, b, similarity)
				case sameKey && similarity < 0.2:
					add("easy-negative", a, b, similarity)
				}
			}
		}
	}

	var items []Item
	counts := map[string]int{}
	for _, band := range D5Bands {
		list := bands[band.Name]
		mix(list, seed+band.Name)
		// One company can't dominate a band.
		perCompany := map[string]int{}
		for _, item := range list {
			if counts[band.Name] == band.Count {
				break
			}
			company := dedupe.Key(item.A.Company, "")
			if perCompany[company] >= 4 {
				continue
			}
			perCompany[company]++
			counts[band.Name]++
			items = append(items, item)
		}
		counts["available:"+band.Name] = len(list)
	}
	mix(items, seed)
	return items, counts, nil
}

func cutKey(key string) (string, string, bool) {
	for i := range key {
		if key[i] == '|' {
			return key[:i], key[i+1:], true
		}
	}
	return key, "", false
}
