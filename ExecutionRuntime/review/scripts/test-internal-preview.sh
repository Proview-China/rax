#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
first="${work_dir}/first"
second="${work_dir}/second"
tampered="${work_dir}/tampered"
malicious="${work_dir}/malicious"
launcher="${work_dir}/launcher"
mkdir -p "${first}" "${second}" "${tampered}" "${malicious}" "${launcher}"

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

token='0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'
cursor='abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789'
run_launcher() {
  PRAXIS_REVIEW_TOKEN="${token}" \
    PRAXIS_REVIEW_CURSOR_KEY_HEX="${cursor}" \
    PRAXIS_REVIEW_DB="$1" \
    PRAXIS_REVIEW_SERVICE_BIN=/bin/true \
    "${script_dir}/run-local.sh"
}

(
  umask 022
  run_launcher "${launcher}/fresh.db"
)
if [[ "$(stat -c '%a' -- "${launcher}/fresh.db")" != 600 ]]; then
  printf '%s\n' 'fresh Review database was not created with mode 0600' >&2
  exit 1
fi

: >"${launcher}/broad.db"
chmod 0644 "${launcher}/broad.db"
if run_launcher "${launcher}/broad.db" >/dev/null 2>&1; then
  printf '%s\n' 'existing broad-permission Review database was accepted' >&2
  exit 1
fi
if [[ "$(stat -c '%a' -- "${launcher}/broad.db")" != 644 ]]; then
  printf '%s\n' 'launcher silently changed an unsafe existing database' >&2
  exit 1
fi

: >"${launcher}/symlink-target.db"
chmod 0600 "${launcher}/symlink-target.db"
ln -s "${launcher}/symlink-target.db" "${launcher}/database-link.db"
if run_launcher "${launcher}/database-link.db" >/dev/null 2>&1; then
  printf '%s\n' 'symlinked Review database was accepted' >&2
  exit 1
fi

mkdir "${launcher}/real-parent"
ln -s "${launcher}/real-parent" "${launcher}/linked-parent"
if run_launcher "${launcher}/linked-parent/review.db" >/dev/null 2>&1; then
  printf '%s\n' 'Review database below a symlinked parent was accepted' >&2
  exit 1
fi
if [[ -e "${launcher}/real-parent/review.db" ]]; then
  printf '%s\n' 'symlinked parent rejection leaked a Review database' >&2
  exit 1
fi

make_malicious_archive() {
  local kind="$1"
  local directory="${malicious}/${kind}"
  mkdir -p "${directory}"
  python3 - "${kind}" "${directory}/malicious.tar.gz" <<'PY'
import io
import sys
import tarfile

kind, output = sys.argv[1:3]
root = "praxis-review-internal-preview"
with tarfile.open(output, "w:gz") as archive:
    member = tarfile.TarInfo()
    data = b"malicious"
    if kind == "traversal":
        member.name = "../../outside-review-preview"
    elif kind == "absolute":
        member.name = "/tmp/outside-review-preview"
    elif kind == "symlink":
        member.name = f"{root}/bin/review-service"
        member.type = tarfile.SYMTYPE
        member.linkname = "/bin/true"
        data = b""
    elif kind == "hardlink":
        member.name = f"{root}/bin/review-service"
        member.type = tarfile.LNKTYPE
        member.linkname = f"{root}/manifest.json"
        data = b""
    elif kind == "unexpected":
        member.name = f"{root}/unexpected"
    else:
        raise SystemExit(f"unknown malicious archive kind: {kind}")
    member.mode = 0o755
    member.size = len(data)
    archive.addfile(member, io.BytesIO(data))
PY
  (
    cd "${directory}"
    sha256sum malicious.tar.gz >malicious.tar.gz.sha256
  )
}

for kind in traversal absolute symlink hardlink unexpected; do
  make_malicious_archive "${kind}"
  if "${script_dir}/verify-internal-preview.sh" \
    "${malicious}/${kind}/malicious.tar.gz" >/dev/null 2>&1; then
    printf 'malicious %s archive was accepted\n' "${kind}" >&2
    exit 1
  fi
done

printf '%s\n' 'internal-preview reproducibility/blackbox/fault gate: PASS'
