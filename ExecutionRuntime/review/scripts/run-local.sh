#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
service_bin="${PRAXIS_REVIEW_SERVICE_BIN:-${script_dir}/review-service}"

: "${PRAXIS_REVIEW_TOKEN:?PRAXIS_REVIEW_TOKEN is required}"
: "${PRAXIS_REVIEW_CURSOR_KEY_HEX:?PRAXIS_REVIEW_CURSOR_KEY_HEX is required}"
: "${PRAXIS_REVIEW_DB:?PRAXIS_REVIEW_DB is required}"

tenant="${PRAXIS_REVIEW_TENANT:-local-preview}"
subject="${PRAXIS_REVIEW_SUBJECT:-local-operator}"
export PRAXIS_REVIEW_ADDR="${PRAXIS_REVIEW_ADDR:-127.0.0.1:8087}"
export PRAXIS_REVIEW_AUTH_TTL_SECONDS="${PRAXIS_REVIEW_AUTH_TTL_SECONDS:-3600}"

if [[ ! "${PRAXIS_REVIEW_TOKEN}" =~ ^[[:xdigit:]]{64,4096}$ ]]; then
  printf '%s\n' 'PRAXIS_REVIEW_TOKEN must contain 64-4096 hexadecimal characters' >&2
  exit 2
fi
if [[ ! "${PRAXIS_REVIEW_CURSOR_KEY_HEX}" =~ ^[[:xdigit:]]{64,4096}$ ]]; then
  printf '%s\n' 'PRAXIS_REVIEW_CURSOR_KEY_HEX must contain 64-4096 hexadecimal characters' >&2
  exit 2
fi
if [[ ! "${tenant}" =~ ^[A-Za-z0-9._:-]{1,128}$ ]] || [[ ! "${subject}" =~ ^[A-Za-z0-9._:@+-]{1,128}$ ]]; then
  printf '%s\n' 'PRAXIS_REVIEW_TENANT or PRAXIS_REVIEW_SUBJECT is invalid' >&2
  exit 2
fi
if [[ ! "${PRAXIS_REVIEW_AUTH_TTL_SECONDS}" =~ ^[0-9]+$ ]] ||
  ((PRAXIS_REVIEW_AUTH_TTL_SECONDS < 1 || PRAXIS_REVIEW_AUTH_TTL_SECONDS > 2592000)); then
  printf '%s\n' 'PRAXIS_REVIEW_AUTH_TTL_SECONDS must be between 1 and 2592000' >&2
  exit 2
fi
if [[ ! -x "${service_bin}" ]]; then
  printf 'review-service is not executable: %s\n' "${service_bin}" >&2
  exit 2
fi

capabilities='["review.attest","review.behavior-feedback.create","review.cancel","review.claim","review.evidence.attach","review.finding.create","review.read","review.submit"]'
export PRAXIS_REVIEW_AUTH_JSON
PRAXIS_REVIEW_AUTH_JSON="$(printf \
  '{"entries":[{"token":"%s","tenant_id":"%s","subject_id":"%s","capabilities":%s}],"ttl_seconds":%s}' \
  "${PRAXIS_REVIEW_TOKEN}" "${tenant}" "${subject}" "${capabilities}" "${PRAXIS_REVIEW_AUTH_TTL_SECONDS}")"

exec "${service_bin}"
