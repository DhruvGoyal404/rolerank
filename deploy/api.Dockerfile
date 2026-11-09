# The API and aiworker: one image, two commands (deploy/compose.prod.yaml). Built in the cloud
# by .github/workflows/deploy.yml from the repository root; nothing local, no .env.
# Base pinned by digest, published 2026-10-21 (the 14-day rule).
FROM node:24.22.0-bookworm-slim@sha256:91acba91b842bec0bf7ea6f5e09c644c7ccad72c4996ed9616d861d1c7cd60d4 AS build
WORKDIR /app
COPY api/package.json api/package-lock.json ./
RUN npm ci --ignore-scripts --no-fund --no-audit
COPY api/tsconfig.json api/tsconfig.build.json ./
COPY api/src ./src
RUN npx tsc -p tsconfig.build.json

FROM node:24.22.0-bookworm-slim@sha256:91acba91b842bec0bf7ea6f5e09c644c7ccad72c4996ed9616d861d1c7cd60d4
WORKDIR /app
COPY api/package.json api/package-lock.json ./
RUN npm ci --omit=dev --ignore-scripts --no-fund --no-audit && npm cache clean --force
COPY --from=build /app/dist ./dist
# The skill taxonomy is shared with the Go worker; the API reads it at start-up.
COPY worker/internal/skills/taxonomy.txt ./taxonomy.txt
ENV NODE_ENV=production ROLERANK_TAXONOMY=/app/taxonomy.txt
USER node
CMD ["node", "dist/index.js"]
