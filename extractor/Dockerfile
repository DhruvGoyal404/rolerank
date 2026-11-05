# The resume extractor's sandbox image. It runs with no network, a read-only root, as nobody,
# under gVisor in production (worker/internal/extract). Base pinned by digest, 2026-10-21.
FROM node:24.22.0-bookworm-slim@sha256:91acba91b842bec0bf7ea6f5e09c644c7ccad72c4996ed9616d861d1c7cd60d4
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --omit=optional --ignore-scripts --no-fund --no-audit && npm cache clean --force
COPY extract.mjs ./
USER 65534:65534
ENV NODE_OPTIONS="--max-old-space-size=384 --disallow-code-generation-from-strings"
ENTRYPOINT ["node", "/app/extract.mjs"]
