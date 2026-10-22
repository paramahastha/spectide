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
cleanup() {
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
measurement="$(python3 - "${binary}" "${state_db}" "${server_log}" <<'PY'
import subprocess
import sys
import time
import urllib.error
import urllib.request

binary, database, log_path = sys.argv[1:]
started = time.monotonic_ns()
with open(log_path, "w", encoding="utf-8") as log:
    process = subprocess.Popen([binary, "--db", database, "serve", "--port", "0"], stdout=log, stderr=subprocess.STDOUT)
try:
    address = ""
    deadline = time.monotonic() + 2
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise SystemExit("health server exited before becoming ready")
        try:
            with open(log_path, "r", encoding="utf-8") as output:
                for line in output:
                    marker = "http://"
                    if marker in line:
                        address = line.split(marker, 1)[1].strip()
                        break
            if address:
                with urllib.request.urlopen(f"http://{address}/healthz", timeout=0.05) as response:
                    if response.status == 200:
                        elapsed = time.monotonic_ns() - started
                        print(f"{address}\t{elapsed / 1_000_000:.3f}")
                        if elapsed > 2_000_000_000:
                            raise SystemExit("health endpoint startup exceeded the 2s threshold")
                        break
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(0.01)
    else:
        raise SystemExit("health endpoint did not respond during the 2s startup window")
finally:
    process.terminate()
    try:
        process.wait(timeout=2)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
PY
)"
address="${measurement%%$'\t'*}"
elapsed_ms="${measurement#*$'\t'}"
printf 'Build command: make build (go build -o bin/spectide ./cmd/spectide)\n'
printf 'Process start to first HTTP 200 /healthz: %s ms (monotonic clock)\n' "${elapsed_ms}"
echo "health endpoint ready within the 2s startup threshold"
output="$("${binary}" --db "${state_db}" demo --project "${repo_root}/examples/fake-provider-demo")"
printf '%s\n' "${output}"
grep -q 'SIMULATED run completed' <<<"${output}"
