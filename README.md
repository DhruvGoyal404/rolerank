# RoleRank

RoleRank ranks real job postings from legal, public sources against your resume and
explains every match: which skills fit, which are missing, and whether the role suits
your level and location. It's built first for freshers and interns in India.

**Status: Phase 1 (foundation).** It isn't usable yet. Plan and dates:
[docs/ROADMAP.md](docs/ROADMAP.md). Project rules: [CLAUDE.md](CLAUDE.md).

## Local development

Needs Docker, Node 24.22.0 and Go.

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

## Bot

RoleRank's crawler identifies itself as `RoleRankBot`, obeys robots.txt and makes at
most 1 request per second per host. Contact: bot@dhruvgoyal.tech.

## License

[MIT](LICENSE).
