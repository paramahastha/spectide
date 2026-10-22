#!/usr/bin/env bash
set -euo pipefail

pattern='(-----BEGIN [A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-ant-api03-[A-Za-z0-9_-]{30,}|sk-proj-[A-Za-z0-9_-]{30,})'
if git grep -I -n -E "${pattern}" -- . >/dev/null 2>&1; then
  echo "tracked content contains a credential-shaped value; inspect locally without printing it into CI logs" >&2
  exit 1
else
  status=$?
  if [[ "${status}" -ne 1 ]]; then
    echo "secret scan could not inspect the Git index" >&2
    exit "${status}"
  fi
fi
echo "tracked content contains no recognized private-key or provider-token patterns"
