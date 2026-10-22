#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
binary="${repo_root}/bin/spectide"
if [[ ! -x "${binary}" ]]; then
  echo "missing ${binary}; run make build first" >&2
  exit 1
fi

state_db="$(mktemp "${TMPDIR:-/tmp}/spectide-smoke.XXXXXX")"
trap 'rm -f "${state_db}" "${state_db}-wal" "${state_db}-shm"' EXIT
"${binary}" doctor
output="$("${binary}" --db "${state_db}" demo --project "${repo_root}/examples/fake-provider-demo")"
printf '%s\n' "${output}"
grep -q 'SIMULATED run completed' <<<"${output}"
