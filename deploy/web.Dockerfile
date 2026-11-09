# Caddy with the built site and deploy/Caddyfile. Built in the cloud by
# .github/workflows/deploy.yml from the repository root. Bases pinned by digest: node published
# 2026-10-21, caddy 2.11.6 published 2026-10-04 (the 14-day rule).
FROM node:24.22.0-bookworm-slim@sha256:91acba91b842bec0bf7ea6f5e09c644c7ccad72c4996ed9616d861d1c7cd60d4 AS build
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts --no-fund --no-audit
COPY web/ ./
# Client build, then the public pages prerendered to HTML (web/scripts/prerender.mjs).
RUN npm run build

FROM caddy:2.11.6-alpine@sha256:d44355d3c2149dc580ce2cac735955d1c08d3d00882c30489c241aa51a5c10d9
COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY --from=build /app/dist /srv/web
