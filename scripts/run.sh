#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

need_build=0
if [ ! -x bluetooth-widget ]; then
  need_build=1
elif find . -name '*.go' -newer bluetooth-widget 2>/dev/null | grep -q .; then
  need_build=1
elif [ go.mod -nt bluetooth-widget ]; then
  need_build=1
fi

if [ "$need_build" -eq 1 ]; then
  make build
fi

exec ./bluetooth-widget "$@"
