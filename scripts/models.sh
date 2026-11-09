#!/usr/bin/env bash
# Downloads the pinned embedding model and ONNX Runtime into ./models (gitignored) and checks
# every file's SHA-256. Safe to re-run: a file that already matches isn't fetched again.
set -euo pipefail
export MSYS_NO_PATHCONV=1
# ROLERANK_ROOT: the image build (deploy/worker.Dockerfile) has no git checkout.
cd "${ROLERANK_ROOT:-$(git rev-parse --show-toplevel)}"
UA="RoleRankBot/0.1 (+https://rolerank.dhruvgoyal.tech/about/bot; bot@dhruvgoyal.tech)"
REV=5c38ec7c405ec4b44b94cc5a9bb96e735b38267a
ORT=1.31.0
arch=${1:-x64}

fetch() { # url path sha256
  if [ -f "$2" ] && echo "$3  $2" | sha256sum -c --status; then return; fi
  mkdir -p "$(dirname "$2")"
  curl -fsSL --retry 3 -A "$UA" -o "$2.part" "$1"
  echo "$3  $2.part" | sha256sum -c --status || { echo "checksum mismatch: $2" >&2; rm -f "$2.part"; exit 1; }
  mv "$2.part" "$2"
}

hf=https://huggingface.co/BAAI/bge-small-en-v1.5/resolve/$REV
fetch "$hf/onnx/model.onnx" models/bge-small-en-v1.5/model.onnx 828e1496d7fabb79cfa4dcd84fa38625c0d3d21da474a00f08db0f559940cf35
fetch "$hf/vocab.txt" models/bge-small-en-v1.5/vocab.txt 07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3

case "$arch" in
  x64) sum=cc5c72baf5ae5c8238a6841f0897227be2d02826b9cf98eaf02fdefaeeb03a57 ;;
  aarch64) sum=c5b8b3cca31f3d643a3b313b8f42e2d0f16a4fcefd02cda343d97bd5306afaab ;;
  *) echo "arch is x64 or aarch64" >&2; exit 1 ;;
esac
tgz=models/onnxruntime/onnxruntime-linux-$arch-$ORT.tgz
fetch "https://github.com/microsoft/onnxruntime/releases/download/v$ORT/onnxruntime-linux-$arch-$ORT.tgz" "$tgz" "$sum"
# The archive's symlinks can't be made on Windows; the versioned .so is all we load.
tar -xzf "$tgz" -C models/onnxruntime --exclude='*.so' --exclude='*.so.1'
echo "models ready"
