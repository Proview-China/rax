#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
first="${work_dir}/first"
second="${work_dir}/second"
tampered="${work_dir}/tampered"
mkdir -p "${first}" "${second}" "${tampered}"

PRAXIS_REVIEW_ALLOW_DIRTY=1 "${script_dir}/package-internal-preview.sh" "${first}" >/dev/null
PRAXIS_REVIEW_ALLOW_DIRTY=1 "${script_dir}/package-internal-preview.sh" "${second}" >/dev/null
first_archive="${first}/praxis-review-internal-preview_linux_amd64.tar.gz"
second_archive="${second}/praxis-review-internal-preview_linux_amd64.tar.gz"
if ! cmp -s "${first_archive}" "${second_archive}"; then
  printf '%s\n' 'internal-preview archives are not reproducible' >&2
  exit 1
fi

"${script_dir}/verify-internal-preview.sh" "${first_archive}"

cp "${first_archive}" "${tampered}/$(basename -- "${first_archive}")"
cp "${first_archive}.sha256" "${tampered}/$(basename -- "${first_archive}").sha256"
printf 'tampered' >>"${tampered}/$(basename -- "${first_archive}")"
if "${script_dir}/verify-internal-preview.sh" "${tampered}/$(basename -- "${first_archive}")" >/dev/null 2>&1; then
  printf '%s\n' 'tampered internal-preview archive was accepted' >&2
  exit 1
fi

if PRAXIS_REVIEW_TOKEN=not-hex \
  PRAXIS_REVIEW_CURSOR_KEY_HEX=abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789 \
  PRAXIS_REVIEW_DB="${work_dir}/invalid.db" \
  PRAXIS_REVIEW_SERVICE_BIN=/bin/true \
  "${script_dir}/run-local.sh" >/dev/null 2>&1; then
  printf '%s\n' 'invalid internal-preview bearer token was accepted' >&2
  exit 1
fi

printf '%s\n' 'internal-preview reproducibility/blackbox/fault gate: PASS'
