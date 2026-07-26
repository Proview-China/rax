#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
service_bin="${PRAXIS_REVIEW_SERVICE_BIN:-${script_dir}/review-service}"

: "${PRAXIS_REVIEW_TOKEN:?PRAXIS_REVIEW_TOKEN is required}"
: "${PRAXIS_REVIEW_CURSOR_KEY_HEX:?PRAXIS_REVIEW_CURSOR_KEY_HEX is required}"
: "${PRAXIS_REVIEW_DB:?PRAXIS_REVIEW_DB is required}"

tenant="${PRAXIS_REVIEW_TENANT:-local-preview}"
subject="${PRAXIS_REVIEW_SUBJECT:-local-operator}"
export PRAXIS_REVIEW_ADDR="${PRAXIS_REVIEW_ADDR:-127.0.0.1:8087}"
export PRAXIS_REVIEW_AUTH_TTL_SECONDS="${PRAXIS_REVIEW_AUTH_TTL_SECONDS:-3600}"
export PRAXIS_REVIEW_MODE="${PRAXIS_REVIEW_MODE:-serve}"

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
if [[ "${PRAXIS_REVIEW_MODE}" != serve && "${PRAXIS_REVIEW_MODE}" != check ]]; then
  printf '%s\n' 'PRAXIS_REVIEW_MODE must be serve or check' >&2
  exit 2
fi
if [[ ! -x "${service_bin}" ]]; then
  printf 'review-service is not executable: %s\n' "${service_bin}" >&2
  exit 2
fi

fail_db_path() {
  printf 'unsafe PRAXIS_REVIEW_DB: %s\n' "$1" >&2
  exit 2
}

validate_private_file() {
  local path="$1"
  local label="$2"
  [[ ! -L "${path}" ]] || fail_db_path "${label} is a symlink"
  [[ -f "${path}" ]] || fail_db_path "${label} is not a regular file"
  [[ "$(stat -c '%u' -- "${path}")" == "$(id -u)" ]] ||
    fail_db_path "${label} is not owned by the current user"
  [[ "$(stat -c '%a' -- "${path}")" == 600 ]] ||
    fail_db_path "${label} permissions must already be 0600"
  [[ "$(stat -c '%h' -- "${path}")" == 1 ]] ||
    fail_db_path "${label} must not have hard links"
}

validate_db_path() {
  local db="$1"
  local parent component segment sidecar
  [[ "${db}" == /* ]] || fail_db_path "path must be absolute"
  [[ "${db}" != "/" && "${db}" != */ && "${db}" != *//* ]] ||
    fail_db_path "path is not canonical"
  [[ "${db}" != */./* && "${db}" != */../* ]] ||
    fail_db_path "dot path components are forbidden"

  parent="${db%/*}"
  [[ -n "${parent}" ]] || parent="/"
  component="/"
  IFS='/' read -r -a segments <<<"${parent#/}"
  for segment in "${segments[@]}"; do
    [[ -n "${segment}" ]] || continue
    component="${component%/}/${segment}"
    [[ ! -L "${component}" ]] || fail_db_path "parent path contains a symlink"
    [[ -d "${component}" ]] || fail_db_path "parent path is not an existing directory"
  done

  if [[ -L "${db}" ]]; then
    fail_db_path "database path is a symlink"
  elif [[ -e "${db}" ]]; then
    validate_private_file "${db}" "existing database"
  else
    (set -o noclobber; : >"${db}") 2>/dev/null ||
      fail_db_path "database could not be created exclusively"
    validate_private_file "${db}" "fresh database"
  fi

  for sidecar in "${db}-wal" "${db}-shm" "${db}-journal"; do
    if [[ -L "${sidecar}" ]]; then
      fail_db_path "database sidecar is a symlink"
    elif [[ -e "${sidecar}" ]]; then
      validate_private_file "${sidecar}" "existing database sidecar"
    fi
  done
}

validate_db_path "${PRAXIS_REVIEW_DB}"

capabilities='["review.attest","review.behavior-feedback.create","review.cancel","review.claim","review.evidence.attach","review.finding.create","review.read","review.submit"]'
export PRAXIS_REVIEW_AUTH_JSON
PRAXIS_REVIEW_AUTH_JSON="$(printf \
  '{"entries":[{"token":"%s","tenant_id":"%s","subject_id":"%s","capabilities":%s}],"ttl_seconds":%s}' \
  "${PRAXIS_REVIEW_TOKEN}" "${tenant}" "${subject}" "${capabilities}" "${PRAXIS_REVIEW_AUTH_TTL_SECONDS}")"

exec "${service_bin}"
