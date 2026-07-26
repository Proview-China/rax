#!/usr/bin/env bash
set -euo pipefail

if (($# != 1)); then
  printf 'usage: %s <internal-preview.tar.gz>\n' "$0" >&2
  exit 2
fi

archive="$(cd -- "$(dirname -- "$1")" && pwd)/$(basename -- "$1")"
checksum="${archive}.sha256"
for command in curl python3 sha256sum tar; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'required command is unavailable: %s\n' "${command}" >&2
    exit 2
  fi
done
if [[ ! -f "${archive}" || ! -f "${checksum}" ]]; then
  printf '%s\n' 'archive or checksum file is missing' >&2
  exit 2
fi
(cd "$(dirname -- "${archive}")" && sha256sum -c "$(basename -- "${checksum}")")

work_dir="$(mktemp -d)"
service_pid=
cleanup() {
  if [[ -n "${service_pid}" ]] && kill -0 "${service_pid}" 2>/dev/null; then
    kill -TERM "${service_pid}" 2>/dev/null || true
    wait "${service_pid}" 2>/dev/null || true
  fi
  rm -rf "${work_dir}"
}
trap cleanup EXIT
tar -xzf "${archive}" -C "${work_dir}"
root="${work_dir}/praxis-review-internal-preview"
for path in bin/review-service bin/praxis-review bin/run-local.sh docs/README.md docs/INTERNAL_PREVIEW.md manifest.json; do
  if [[ ! -e "${root}/${path}" ]]; then
    printf 'package entry is missing: %s\n' "${path}" >&2
    exit 1
  fi
done
if ! grep -q '"channel": "internal-preview"' "${root}/manifest.json" ||
  ! grep -q '"support_mode": "owner-local"' "${root}/manifest.json" ||
  ! grep -q '"production_eligible": false' "${root}/manifest.json"; then
  printf '%s\n' 'package manifest boundary is invalid' >&2
  exit 1
fi

port="$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)"
token='0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'
cursor='abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789'
export PRAXIS_REVIEW_TOKEN="${token}"
export PRAXIS_REVIEW_CURSOR_KEY_HEX="${cursor}"
export PRAXIS_REVIEW_DB="${work_dir}/review.db"
export PRAXIS_REVIEW_ADDR="127.0.0.1:${port}"
export PRAXIS_REVIEW_TENANT="internal-preview"
export PRAXIS_REVIEW_SERVICE_BIN="${root}/bin/review-service"
"${root}/bin/run-local.sh" >"${work_dir}/service.stdout" 2>"${work_dir}/service.stderr" &
service_pid=$!

base="http://${PRAXIS_REVIEW_ADDR}"
ready=false
for _ in $(seq 1 100); do
  if ! kill -0 "${service_pid}" 2>/dev/null; then
    printf '%s\n' 'review-service exited before becoming reachable' >&2
    sed -n '1,120p' "${work_dir}/service.stderr" >&2
    exit 1
  fi
  code="$(curl --silent --output /dev/null --write-out '%{http_code}' \
    "${base}/v1/reviews?tenant=internal-preview" || true)"
  if [[ "${code}" == 401 ]]; then
    ready=true
    break
  fi
  sleep 0.05
done
if [[ "${ready}" != true ]]; then
  printf '%s\n' 'review-service did not expose the authenticated API boundary' >&2
  exit 1
fi

export PRAXIS_REVIEW_URL="${base}"
cli_output="$("${root}/bin/praxis-review" list --tenant internal-preview)"
if ! grep -Fq '"cases": []' <<<"${cli_output}"; then
  printf '%s\n' 'praxis-review CLI did not return the empty Review collection' >&2
  printf '%s\n' "${cli_output}" >&2
  exit 1
fi
if [[ ! -s "${PRAXIS_REVIEW_DB}" ]]; then
  printf '%s\n' 'review SQLite database was not created' >&2
  exit 1
fi

kill -TERM "${service_pid}"
wait "${service_pid}"
service_pid=
printf '%s\n' 'internal-preview verification: PASS'
