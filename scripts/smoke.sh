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
os_name="$(uname -s)"
case "${os_name}" in
  Darwin)
    os_version="macOS $(sw_vers -productVersion)"
    cpu_model="$(sysctl -n machdep.cpu.brand_string)"
    memory_bytes="$(sysctl -n hw.memsize)"
    ;;
  Linux)
    os_version="$(uname -sr)"
    cpu_model="$(awk -F: '/model name/ {sub(/^[[:space:]]+/, "", $2); print $2; exit}' /proc/cpuinfo)"
    memory_bytes="$(awk '/MemTotal/ {print $2 * 1024; exit}' /proc/meminfo)"
    ;;
  *)
    echo "unsupported verification host: ${os_name}" >&2
    exit 1
    ;;
esac
architecture="$(uname -m)"
memory_gib="$(python3 -c 'import sys; print(f"{int(sys.argv[1]) / (1024 ** 3):.1f}")' "${memory_bytes}")"
echo "Verification host: ${os_version}; CPU: ${cpu_model}; RAM: ${memory_gib} GiB; architecture: ${architecture}"
started_ns="$(python3 -c 'import time; print(time.monotonic_ns())')"
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
finished_ns="$(python3 -c 'import time; print(time.monotonic_ns())')"
elapsed_ms="$(python3 -c 'import sys; print(f"{(int(sys.argv[2]) - int(sys.argv[1])) / 1_000_000:.3f}")' "${started_ns}" "${finished_ns}")"
printf 'Build command: make build (go build -o bin/spectide ./cmd/spectide)\n'
printf 'Process start to first HTTP 200 /healthz: %s ms (monotonic clock)\n' "${elapsed_ms}"
if python3 -c 'import sys; raise SystemExit(int(int(sys.argv[2]) - int(sys.argv[1]) > 2_000_000_000))' "${started_ns}" "${finished_ns}"; then
  :
else
  echo "health endpoint startup exceeded the 2s threshold" >&2
  exit 1
fi
echo "health endpoint ready within the 2s startup threshold"
output="$("${binary}" --db "${state_db}" demo --project "${repo_root}/examples/fake-provider-demo")"
printf '%s\n' "${output}"
grep -q 'SIMULATED run completed' <<<"${output}"
