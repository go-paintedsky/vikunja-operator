#!/usr/bin/env bash
# Lint commit messages with the pinned Go commitlint binary and .commitlint.yaml.
# Usage:
#   hack/commitlint.sh <commit-message-file>   leftover commit-msg
#   hack/commitlint.sh                         read message from stdin
#   hack/commitlint.sh --from <sha> --to <sha> lint each commit in a range
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${root}"

make -s commitlint

bin="${root}/bin/commitlint"
config="${root}/.commitlint.yaml"

lint_stdin() {
  "${bin}" lint --config "${config}"
}

lint_file() {
  "${bin}" lint --config "${config}" --message "$1"
}

lint_range() {
  local from="$1" to="$2" sha
  while read -r sha; do
    echo "Linting commit ${sha}"
    git log -1 --format='%B' "${sha}" | lint_stdin
  done < <(git log --reverse --format='%H' "${from}..${to}")
}

if [[ $# -eq 0 ]]; then
  lint_stdin
  exit 0
fi

if [[ $# -eq 1 && "$1" != --* ]]; then
  lint_file "$1"
  exit 0
fi

from=""
to=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --from)
      from="${2:-}"
      shift 2
      ;;
    --to)
      to="${2:-}"
      shift 2
      ;;
    *)
      echo "usage: hack/commitlint.sh [<commit-message-file>|--from <sha> --to <sha>]" >&2
      exit 1
      ;;
  esac
done

if [[ -z "${from}" || -z "${to}" ]]; then
  echo "usage: hack/commitlint.sh --from <sha> --to <sha>" >&2
  exit 1
fi

lint_range "${from}" "${to}"
