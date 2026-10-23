#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"
version="${1:-}"
if [[ ! "${version}" =~ ^[0-9A-Za-z][0-9A-Za-z.+_-]*$ ]]; then
  echo "usage: $0 VERSION (use a release version such as 1.0.0 or a prerelease label)" >&2
  exit 2
fi

static_root="${repo_root}/internal/webassets/static"
if [[ ! -s "${static_root}/index.html" ]] || ! compgen -G "${static_root}/assets/*.js" >/dev/null; then
  echo "embedded dashboard is missing; run make web-build first" >&2
  exit 1
fi

release_dir="${repo_root}/dist/release"
rm -rf "${release_dir}"
mkdir -p "${release_dir}"
commit="$(git -C "${repo_root}" rev-parse HEAD)"
if git -C "${repo_root}" diff --quiet && git -C "${repo_root}" diff --cached --quiet; then
  working_tree_clean=true
else
  working_tree_clean=false
fi
go_version="$(go version | awk '{print $3}')"
node_version="$(node --version)"
package_names=()
package_dirs=()
targets=("darwin/arm64" "darwin/amd64" "linux/arm64" "linux/amd64")

for target in "${targets[@]}"; do
  goos="${target%/*}"
  goarch="${target#*/}"
  package="spectide_${version}_${goos}_${goarch}"
  package_dir="${release_dir}/${package}"
  mkdir -p "${package_dir}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" go build \
    -trimpath \
    -ldflags "-s -w -X github.com/spectide/spectide/internal/cli.BuildVersion=${version}" \
    -o "${package_dir}/spectide" ./cmd/spectide
  cp "${repo_root}/README.md" "${repo_root}/LICENSE" "${package_dir}/"
  printf '%s\n' "${version}" > "${package_dir}/VERSION"
  python3 - "${release_dir}/${package}" "${release_dir}/${package}.tar.gz" <<'PY'
import gzip
import os
import sys
import tarfile

root, archive_path = sys.argv[1:]
with open(archive_path, "wb") as raw:
    with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode="w", format=tarfile.PAX_FORMAT) as archive:
            paths = [root]
            for parent, directories, files in os.walk(root):
                directories.sort()
                files.sort()
                paths.extend(os.path.join(parent, name) for name in directories + files)
            for path in paths:
                entry = archive.gettarinfo(path, arcname=os.path.relpath(path, os.path.dirname(root)))
                entry.uid = entry.gid = 0
                entry.uname = entry.gname = ""
                entry.mtime = 0
                if entry.isfile():
                    with open(path, "rb") as source:
                        archive.addfile(entry, source)
                else:
                    archive.addfile(entry)
PY
  package_names+=("${package}.tar.gz")
  package_dirs+=("${package_dir}")
done

python3 - "${release_dir}/release-metadata.json" "${version}" "${commit}" "${go_version}" "${node_version}" "${working_tree_clean}" <<'PY'
import json
import sys

path, version, commit, go_version, node_version, working_tree_clean = sys.argv[1:]
metadata = {
    "product": "SpecTide",
    "version": version,
    "commit": commit,
    "working_tree_clean": working_tree_clean == "true",
    "go_toolchain": go_version,
    "node_toolchain": node_version,
    "dashboard": "embedded",
    "targets": ["darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64"],
    "publication": "not published; local/manual preparation only",
}
with open(path, "w", encoding="utf-8") as output:
    json.dump(metadata, output, indent=2, sort_keys=True)
    output.write("\n")
PY

if command -v sha256sum >/dev/null 2>&1; then
  (cd "${release_dir}" && sha256sum "${package_names[@]}" release-metadata.json > SHA256SUMS)
else
  (cd "${release_dir}" && shasum -a 256 "${package_names[@]}" release-metadata.json > SHA256SUMS)
fi

host_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
host_arch="$(uname -m)"
case "${host_arch}" in
  arm64|aarch64) host_arch=arm64 ;;
  x86_64|amd64) host_arch=amd64 ;;
esac
host_binary="${release_dir}/spectide_${version}_${host_os}_${host_arch}/spectide"
if [[ -x "${host_binary}" ]]; then
  actual_version="$("${host_binary}" version)"
  [[ "${actual_version}" == "spectide ${version}" ]] || { echo "version smoke failed: ${actual_version}" >&2; exit 1; }
  SPECTIDE_BINARY="${host_binary}" "${repo_root}/scripts/smoke.sh"
else
  echo "built host target ${host_os}/${host_arch} is not in the release matrix; cross-target runtime smoke was not run" >&2
fi

echo "Prepared non-published release artifacts in ${release_dir}"
cat "${release_dir}/release-metadata.json"
cat "${release_dir}/SHA256SUMS"
for package_dir in "${package_dirs[@]}"; do
  rm -rf "${package_dir}"
done
