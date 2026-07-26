#!/usr/bin/env bash
set -euo pipefail

if (($# != 1)); then
  printf 'usage: %s <internal-preview.tar.gz>\n' "$0" >&2
  exit 2
fi

archive="$(cd -- "$(dirname -- "$1")" && pwd)/$(basename -- "$1")"
checksum="${archive}.sha256"
for command in curl python3 sha256sum; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'required command is unavailable: %s\n' "${command}" >&2
    exit 2
  fi
done
if [[ ! -f "${archive}" || ! -f "${checksum}" || -L "${archive}" || -L "${checksum}" ]]; then
  printf '%s\n' 'archive or checksum file is missing' >&2
  exit 2
fi

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
input_dir="${work_dir}/input"
extract_dir="${work_dir}/extract"
mkdir -m 0700 "${input_dir}" "${extract_dir}"
cp --no-dereference -- "${archive}" "${input_dir}/$(basename -- "${archive}")"
cp --no-dereference -- "${checksum}" "${input_dir}/$(basename -- "${checksum}")"
archive="${input_dir}/$(basename -- "${archive}")"
checksum="${archive}.sha256"
if [[ ! -f "${archive}" || -L "${archive}" || ! -f "${checksum}" || -L "${checksum}" ]]; then
  printf '%s\n' 'archive snapshot or checksum snapshot is unsafe' >&2
  exit 1
fi
mapfile -t checksum_lines <"${checksum}"
archive_basename="$(basename -- "${archive}")"
if ((${#checksum_lines[@]} != 1)) ||
  [[ ! "${checksum_lines[0]}" =~ ^[0-9a-f]{64}\ \ .+$ ]] ||
  [[ "${checksum_lines[0]:66}" != "${archive_basename}" ]]; then
  printf '%s\n' 'checksum file shape or archive name is invalid' >&2
  exit 1
fi
expected_hash="${checksum_lines[0]:0:64}"
actual_hash="$(sha256sum -- "${archive}")"
actual_hash="${actual_hash%% *}"
if [[ "${actual_hash}" != "${expected_hash}" ]]; then
  printf '%s\n' 'archive checksum mismatch' >&2
  exit 1
fi
printf '%s: OK\n' "${archive_basename}"

python3 - "${archive}" "${extract_dir}" <<'PY'
import os
import shutil
import sys
import tarfile
from pathlib import PurePosixPath

archive, destination = sys.argv[1:3]
root = "praxis-review-internal-preview"
expected = {
    root: ("dir", 0o755),
    f"{root}/bin": ("dir", 0o755),
    f"{root}/docs": ("dir", 0o755),
    f"{root}/bin/review-service": ("file", 0o755),
    f"{root}/bin/praxis-review": ("file", 0o755),
    f"{root}/bin/run-local.sh": ("file", 0o755),
    f"{root}/docs/README.md": ("file", 0o644),
    f"{root}/docs/INTERNAL_PREVIEW.md": ("file", 0o644),
    f"{root}/manifest.json": ("file", 0o644),
}
max_member_size = 128 * 1024 * 1024
max_total_size = 256 * 1024 * 1024

with tarfile.open(archive, mode="r:gz") as package:
    members = package.getmembers()
    seen = set()
    total_size = 0
    for member in members:
        path = PurePosixPath(member.name)
        if (
            member.name.startswith("/")
            or path.is_absolute()
            or any(part in ("", ".", "..") for part in path.parts)
            or member.name not in expected
            or member.name in seen
        ):
            raise SystemExit(f"unsafe or unexpected package member: {member.name!r}")
        seen.add(member.name)
        expected_kind, expected_mode = expected[member.name]
        actual_kind = "dir" if member.isdir() else "file" if member.isfile() else "other"
        if actual_kind != expected_kind or member.mode & 0o777 != expected_mode:
            raise SystemExit(f"invalid package member type or mode: {member.name!r}")
        if member.linkname or member.issym() or member.islnk() or member.isdev():
            raise SystemExit(f"linked or device package member is forbidden: {member.name!r}")
        if member.size < 0 or member.size > max_member_size:
            raise SystemExit(f"package member size is invalid: {member.name!r}")
        total_size += member.size
        if total_size > max_total_size:
            raise SystemExit("package expanded size exceeds the closed limit")
    if seen != set(expected):
        missing = sorted(set(expected) - seen)
        raise SystemExit(f"package members are missing: {missing!r}")

    for name, (kind, mode) in expected.items():
        target = os.path.join(destination, *PurePosixPath(name).parts)
        if kind == "dir":
            os.mkdir(target, mode)
            continue
        source = package.extractfile(package.getmember(name))
        if source is None:
            raise SystemExit(f"package file cannot be read: {name!r}")
        flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
        if hasattr(os, "O_NOFOLLOW"):
            flags |= os.O_NOFOLLOW
        descriptor = os.open(target, flags, mode)
        with source, os.fdopen(descriptor, "wb") as output:
            shutil.copyfileobj(source, output)
PY

root="${extract_dir}/praxis-review-internal-preview"
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
