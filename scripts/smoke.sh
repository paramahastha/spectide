#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
binary="${repo_root}/bin/spectide"
if [[ ! -x "${binary}" ]]; then
  echo "missing ${binary}; run make build first" >&2
  exit 1
fi

state_db="$(mktemp "${TMPDIR:-/tmp}/spectide-smoke.XXXXXX")"
server_log="$(mktemp "${TMPDIR:-/tmp}/spectide-server.XXXXXX")"
server_pid=""
cleanup() {
  if [[ -n "${server_pid}" ]]; then
    kill "${server_pid}" 2>/dev/null || true
    wait "${server_pid}" 2>/dev/null || true
  fi
  rm -f "${state_db}" "${state_db}-wal" "${state_db}-shm" "${server_log}"
}
trap cleanup EXIT
"${binary}" doctor
SECONDS=0
"${binary}" --db "${state_db}" serve --port 0 >"${server_log}" 2>&1 &
server_pid="$!"
health_ok=0
for _ in $(seq 1 15); do
  address="$(sed -n 's#.*http://##p' "${server_log}" | head -n 1)"
  if [[ -n "${address}" ]] && curl --fail --silent --max-time 0.05 "http://${address}/healthz" >/dev/null; then
    health_ok=1
    break
  fi
  sleep 0.05
done
if [[ "${health_ok}" -ne 1 ]]; then
  echo "health endpoint did not respond during smoke startup window" >&2
  cat "${server_log}" >&2
  exit 1
fi
if [[ "${SECONDS}" -gt 2 ]]; then
  echo "health endpoint startup exceeded the 2s threshold" >&2
  exit 1
fi
echo "health endpoint ready within the 2s startup threshold"
output="$("${binary}" --db "${state_db}" demo --project "${repo_root}/examples/fake-provider-demo")"
printf '%s\n' "${output}"
grep -q 'SIMULATED run completed' <<<"${output}"
