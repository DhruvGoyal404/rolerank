# The Go side: ranker (compose), ingest and embed (compose, catch-up), and the extractor service
# binary, which runs on the host under systemd because it starts gVisor sandboxes
# (deploy/host/deploy.sh copies it out). Also carries deploy/ and db/ for the host, so a
# release is entirely in ECR. Built for arm64 in the cloud by .github/workflows/deploy.yml.
# Bases pinned by digest: golang published 2026-10-08, debian 2026-10-21 (the 14-day rule).
FROM golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS build
ENV CGO_ENABLED=1 GOFLAGS=-buildvcs=false GOTOOLCHAIN=auto
WORKDIR /src/worker
COPY worker/go.mod worker/go.sum ./
RUN go mod download
COPY worker/ ./
RUN go build -trimpath -tags onnx -o /out/ranker ./cmd/ranker \
 && go build -trimpath -tags onnx -o /out/embed ./cmd/embed \
 && go build -trimpath -o /out/ingest ./cmd/ingest \
 && go build -trimpath -o /out/extractor ./cmd/extractor
# The embedding model and ONNX Runtime, pinned by revision and SHA-256 in scripts/models.sh.
WORKDIR /src
COPY scripts/models.sh scripts/models.sh
RUN ROLERANK_ROOT=/src bash scripts/models.sh aarch64

FROM debian:bookworm-20261020-slim@sha256:7ff6ba31527b9e336162fa061746c366c4b2032f4d2cff2078bea43f78efe3f8
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/ /usr/local/bin/
COPY --from=build /src/models/bge-small-en-v1.5/ /models/bge-small-en-v1.5/
COPY --from=build /src/models/onnxruntime/onnxruntime-linux-aarch64-1.31.0/lib/libonnxruntime.so.1.31.0 /models/libonnxruntime.so.1.31.0
COPY deploy/ /app/deploy/
COPY db/ /app/db/
ENV ROLERANK_ORT_LIB=/models/libonnxruntime.so.1.31.0 ROLERANK_EMBED_MODEL=/models/bge-small-en-v1.5/model.onnx
USER 65534:65534
CMD ["/usr/local/bin/ranker"]
