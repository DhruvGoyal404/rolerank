# RoleRank

RoleRank ranks real job postings from legal, public sources against your resume and
explains every match: which skills fit, which are missing, and whether the role suits
your level and location. It's built first for freshers and interns in India.

**Status: Phase 6 done (product UI); deployment (P8) next.** Plan and dates:
[docs/ROADMAP.md](docs/ROADMAP.md). Project rules: [CLAUDE.md](CLAUDE.md).

## Local development

Needs Docker, Node 24.22.0 and Go.

**Everything at once**, against your local corpus (Postgres, Valkey, the extractor, aiworker,
the ranker service, the API and the built site):

```sh
bash scripts/dev-up.sh      # prints http://localhost:5173
bash scripts/dev-down.sh    # stops all of it; data stays in the compose volumes
```

Logs are in `.dev/logs/`. The ranker compiles in its container on first start, so allow a
minute or two. Piece by piece:

```sh
cp .env.example .env                 # fill the values; ROLERANK_* are hex
docker compose up -d --wait
docker compose run --rm migrate-app up
docker compose run --rm migrate-corpus up
(cd api && npm ci && npm run build)
bash scripts/dev-api.sh              # API on 127.0.0.1:8080
(cd web && npm ci && npm run dev)    # http://localhost:5173
```

Use `localhost`, not 127.0.0.1: cookies are per host.

| OAuth app | Dev callback URL | Scopes |
|---|---|---|
| Google | `http://localhost:5173/api/auth/google/callback` | `openid email` |
| GitHub | `http://localhost:5173/api/auth/github/callback` | `read:user` |

## Running ingestion

```sh
bash scripts/ingest.sh boards greenhouse data/greenhouse.txt   # once per source
bash scripts/ingest.sh run                                     # Ctrl-C any time
bash scripts/ingest.sh status
bash scripts/ingest.sh dedupe
bash scripts/ingest.sh deny greenhouse <token>                # employer opt-out, within 7 days
bash scripts/ingest.sh purge <source> <host>                  # after its adapter is removed
```

`run` is safe to stop and rerun at any moment. Board lists come from `go run ./cmd/discover
../data` in `worker/`. A run stopped hard continues its in-flight board after 5 minutes.

## Bot

RoleRank's crawler identifies itself as `RoleRankBot`, obeys robots.txt and makes at
most 1 request per second per host. Contact: bot@dhruvgoyal.tech.

## License

[MIT](LICENSE).
