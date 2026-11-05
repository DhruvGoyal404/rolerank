# Phase 3: search, ranking and evals

## What was built

- **Skills.** A taxonomy of 410 skills with aliases, written by us, and a deterministic matcher.
  It splits required from nice-to-have skills by section and inline markers.
- **Enrich.** `ingest enrich` keeps `search_postings` in step with the corpus: one row per job,
  with the cluster's union of countries and remote scopes. It clears a vector when the text
  changes. On the dev corpus, 21,279 jobs are searchable.
- **Embeddings.** BAAI/bge-small-en-v1.5, pinned by revision and SHA-256, runs through ONNX
  Runtime 1.31.0 in a project-labelled Linux container. The WordPiece tokenizer is pure Go and
  matches the reference token for token. Embedding all 21,279 jobs took 4 h 22 m on this laptop.
- **Ranking.** Hard filters come first: the level window, which always includes unknown-level
  jobs, and geography. Then exact vector search and skill/title overlap retrieval run, fused by
  RRF (k = 60), and a linear rerank over explainable features produces Strong, Good or Stretch.
  Matched and missing skills come without an LLM.
- **Above 50,000 rows,** search switches plan: a partial HNSW index over India-open rows, with
  iterative scan, and no lexical retriever (ARCHITECTURE.md, "Two plans").
- **Evals.** D2 (synthetic, rule-graded, run in CI) and D3 (20 personas on the real corpus) are
  scored by `rank score`, and `rank fit` fits the weights and bands. `bench` loads a 5 lakh
  synthetic corpus and times 20 concurrent users.

## Results

| Criterion | Measured | Threshold | Verdict |
|---|---|---|---|
| D2 hybrid nDCG@10 / MRR / Recall | 0.860 / 1.000 / 1.000 | 0.85 / 0.90 / 0.98 | Pass |
| D2 level / geography violations | 0 / 0 | ≤ 1% / 0 | Pass |
| D3 hybrid nDCG@10 | 0.787 (cross-validated fit 0.78 to 0.80) | 0.75 | Pass |
| D3 hybrid MRR | 0.770 | 0.70 | Pass |
| D3 Recall@200 (retrieval) | 0.985 | 0.90 | Pass |
| D3 level violation@10 | 0 | ≤ 3% | Pass |
| Hybrid beats lexical-only, D3 nDCG@10 | +0.090, 95% interval 0.023 to 0.159 | above 0 | Pass |
| Hybrid beats vector-only | +0.099, 95% interval 0.023 to 0.189 | above 0 | Pass |
| p95, 5 lakh rows, 20 users, laptop | 169 ms with no writes; 188 to 217 ms with writes | < 200 ms on the target host | **Borderline; the host is measured in P8** |
| Filtered recall@20, HNSW against exact | 0.961 | ≥ 0.95 | Pass |
| Bench timer against `EXPLAIN ANALYZE` | 166 ms against 151 ms | agree | Pass |
| Index rebuild | Real corpus, every search index: 42 s. Bench partial HNSW: 23 s. Bench whole-table HNSW: 55 min, while embedding ran | fits up/down | Pass for the real corpus |
| Rank reproducible | Same search twice, same order and scores (`TestD2`) | | Pass |
| Job skill extraction precision | 0.896 (620 of 692, fresh sample) | 0.90 | **Fails, known gap** |

D3 per segment, hybrid nDCG@10: experienced 0.846, fresher 0.672, India-only 0.770, intern
0.810, remote 0.881. Freshers are the weakest. On two segments a baseline beats the hybrid:
vector-only for freshers (0.703) and lexical-only for India-only profiles (0.807). With four
or five profiles per segment, neither difference is sized.

## Decisions made in this phase

- **Profiles use bge's query prefix.** On D3 it lifts hybrid nDCG@10 from 0.773 to 0.787. It
  also turns the lexical comparison from not proven (interval -0.003 to 0.152) into a pass.
- **Weights v1, fitted on D3.** Fitted on D3 alone, the weights gave skill coverage no weight
  and failed D2 (0.721). A floor on the required-skill weight, scaled to sum to 1, passes both.
  Weights: required 0.177, nice 0.013, cosine 0.354, title 0.051, level 0.076, location 0.253,
  fresh 0, fusion 0.076. Bands: strong ≥ 0.505, good ≥ 0.46.
- **Two plans by corpus size.** The numbers that forced this are in ARCHITECTURE.md.

## D3 audit (owner, 2026-11-05)

The owner graded 30 of the D3 pairs: 15 of the hard ones opus had regraded, and 15 that
sonnet graded with high confidence.

| Grades | Agreed with the owner | Cohen's kappa | Quadratic-weighted kappa |
|---|---|---|---|
| Rubric v1 (the grades above) | 19 of 30 | 0.507 (0.288 to 0.721) | 0.831; every disagreement one grade apart |
| Rubric v2, sonnet regrade of all 438 | 19 of 30 | 0.49 | |

Under v1 there were two patterns. The model gave 0 where the owner gave 1 four times: a level
far off, which the v1 rubric's own example called 0. The model also gave 3 where the owner gave
2 four times. Rubric v2 makes any level mismatch a 1 and asks for most core requirements before
a 3, and that fixed the first pattern (one case left). The second grew to six: the owner is
stricter than the model about "would apply today". v2 was written after reading the owner's
answers, so its agreement on these same 30 is flattering, not independent.

**Below the 0.6 bar, D3 can't gate (EVALS.md).** Here is how much the results depend on that
3-or-2 boundary:

| D3 hybrid | v2 grades | Strict: every low-confidence 3 made a 2 | Threshold |
|---|---|---|---|
| nDCG@10 | 0.795 | 0.762 | 0.75 |
| MRR | 0.806 | **0.645** | 0.70 |
| Beats lexical-only | interval 0.012 to 0.148 | **-0.028 to 0.122** | above 0 |
| Beats vector-only | interval 0.016 to 0.168 | **-0.000 to 0.142** | above 0 |

nDCG@10 passes under both. MRR and the comparison rule pass only under the model's reading.
D3 is therefore **not gold**, and its MRR and comparison verdicts are **not proven**. The
ranker ships with v1 weights; v1.1 needs owner-graded pairs, or a rubric the owner and model
agree on at kappa 0.6 or above. `evals/labels/d3-grades-v2.jsonl` holds the v2 grades.

## Known gaps and open items

| Item | State | Next |
|---|---|---|
| Skill extraction precision 0.896 against 0.90 | One alias fix, then re-measured on a fresh sample. Most errors are skills named only about the company or its products | v1.1: skip company-description sections |
| p95 with ingestion writes on this laptop: 188 to 217 ms | Borderline; the laptop isn't the target host | P8 measures it on m7g.xlarge |
| D3 is model-graded, and the owner audit gives kappa 0.49 to 0.51 | Not gold; MRR and the comparison rule are not proven (above) | v1.1: owner-graded pairs, or a rubric agreed at kappa ≥ 0.6 |
| Weights and prefix were chosen with D3 visible | The cross-validated estimate (0.78 to 0.80) is the honest number, not 0.787 | A second D3 draw in v1.1 |
| Cloud embedding candidates (Titan, OpenAI) | Not run, by the owner's decision of 2026-11-05: local bge-small passes and keeps resume data on our host | v1.1 option |
| The large plan has no lexical retriever | Its quality isn't measured on D3, which runs the small plan | Measure if the corpus passes 50,000 rows |
| The bench writer sustains 21 updates a second, not 50 | One connection, serial updates | Enough to show the effect |

## Datasets and labels

- `evals/d3/profiles.json` and `evals/d3/vectors*.jsonl` hold the personas and their vectors.
- `evals/labels/d3-grades.jsonl` holds the IDs, grades and who graded each.
- `evals/labels/skills-model.jsonl` holds the skill check.
- Posting text stays in `evals/private`.
