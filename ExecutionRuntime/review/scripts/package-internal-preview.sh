#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
module_dir="$(cd -- "${script_dir}/.." && pwd)"
repo_dir="$(git -C "${module_dir}" rev-parse --show-toplevel)"
output_dir="${1:-${module_dir}/dist}"
goos="${GOOS:-linux}"
goarch="${GOARCH:-amd64}"
package_name="praxis-review-internal-preview"
archive_name="${package_name}_${goos}_${goarch}.tar.gz"
source_revision="$(git -C "${repo_dir}" rev-parse HEAD)"
source_epoch="${SOURCE_DATE_EPOCH:-$(git -C "${repo_dir}" show -s --format=%ct "${source_revision}")}"
go_version="$(go version | awk '{print $3}')"
dirty=false

if [[ -n "$(git -C "${repo_dir}" status --porcelain=v1 -- ExecutionRuntime/review)" ]]; then
  dirty=true
fi
if [[ "${dirty}" == true && "${PRAXIS_REVIEW_ALLOW_DIRTY:-0}" != 1 ]]; then
  printf '%s\n' 'Review module is dirty; commit it or set PRAXIS_REVIEW_ALLOW_DIRTY=1 for a non-release development build' >&2
  exit 2
fi
if [[ ! "${source_epoch}" =~ ^[0-9]+$ ]] || ((source_epoch <= 0)); then
  printf '%s\n' 'SOURCE_DATE_EPOCH must be a positive integer' >&2
  exit 2
fi
for command in go tar gzip sha256sum install; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'required command is unavailable: %s\n' "${command}" >&2
    exit 2
  fi
done

work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
stage="${work_dir}/${package_name}"
mkdir -p "${stage}/bin" "${stage}/docs" "${output_dir}"

build() {
  local target="$1"
  local package="$2"
  (
    cd "${module_dir}"
    CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
      go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' \
      -o "${target}" "${package}"
  )
}

build "${stage}/bin/review-service" ./cmd/review-service
build "${stage}/bin/praxis-review" ./cmd/praxis-review
install -m 0755 "${module_dir}/scripts/run-local.sh" "${stage}/bin/run-local.sh"
install -m 0644 "${module_dir}/README.md" "${stage}/docs/README.md"
install -m 0644 "${module_dir}/INTERNAL_PREVIEW.md" "${stage}/docs/INTERNAL_PREVIEW.md"

cat >"${stage}/manifest.json" <<EOF
{
  "channel": "internal-preview",
  "support_mode": "owner-local",
  "production_eligible": false,
  "source_revision": "${source_revision}",
  "source_dirty": ${dirty},
  "source_date_epoch": ${source_epoch},
  "go_version": "${go_version}",
  "goos": "${goos}",
  "goarch": "${goarch}"
}
EOF

find "${stage}" -type f -print0 | xargs -0 touch --date="@${source_epoch}"
archive="${output_dir}/${archive_name}"
tar --sort=name --mtime="@${source_epoch}" --owner=0 --group=0 --numeric-owner \
  --format=posix --pax-option=delete=atime,delete=ctime \
  -C "${work_dir}" -cf - "${package_name}" |
  gzip -n >"${archive}"
(cd "${output_dir}" && sha256sum "${archive_name}" >"${archive_name}.sha256")

printf '%s\n' "${archive}"
