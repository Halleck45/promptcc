#!/usr/bin/env sh
# Evaluation harness: scan pinned open-source repositories and compare the
# scan summaries against the snapshots committed in eval/expected/.
#
# Usage:
#   ./eval/run.sh              compare against snapshots, exit 1 on mismatch
#   EVAL_UPDATE=1 ./eval/run.sh  refresh the snapshots (review the diff!)
#
# Repositories are pinned to exact commits so results are reproducible.
# Clones are cached in eval/.cache (git-ignored).
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/promptcc"
CACHE="$ROOT/eval/.cache"
EXPECTED="$ROOT/eval/expected"
UPDATE="${EVAL_UPDATE:-0}"
FAIL=0

if [ ! -x "$BIN" ]; then
    echo "error: $BIN not found, run 'make build' first" >&2
    exit 2
fi
mkdir -p "$CACHE" "$EXPECTED"

fetch() { # fetch <name> <url> <sha>
    name=$1 url=$2 sha=$3
    dir="$CACHE/$name"
    if [ ! -d "$dir/.git" ]; then
        git init -q "$dir"
        git -C "$dir" remote add origin "$url"
    fi
    if ! git -C "$dir" cat-file -e "$sha" 2>/dev/null; then
        echo "fetching $name @ $sha"
        git -C "$dir" fetch -q --depth 1 origin "$sha"
    fi
    git -C "$dir" -c advice.detachedHead=false checkout -qf "$sha"
}

summarize() { # summarize <name>: full per-prompt listing, repo-relative paths
    name=$1
    "$BIN" --verbose "$CACHE/$name" 2>/dev/null \
        | sed "s|$CACHE/$name/||g" \
        | grep -v '^Use --verbose'
}

check() { # check <name> <url> <sha>
    name=$1
    fetch "$@"
    actual="$(summarize "$name")"
    snapshot="$EXPECTED/$name.txt"
    if [ "$UPDATE" = "1" ]; then
        printf '%s\n' "$actual" > "$snapshot"
        echo "updated $snapshot"
        return 0
    fi
    if [ ! -f "$snapshot" ]; then
        echo "MISSING snapshot for $name, run 'make eval-update'" >&2
        FAIL=1
        return 0
    fi
    if printf '%s\n' "$actual" | diff -u "$snapshot" - > /dev/null; then
        echo "OK        $name"
    else
        echo "MISMATCH  $name"
        printf '%s\n' "$actual" | diff -u "$snapshot" - || true
        FAIL=1
    fi
}

check aider https://github.com/Aider-AI/aider.git 5dc9490bb35f9729ef2c95d00a19ccd30c26339c
check cline https://github.com/cline/cline.git c92d4e7553a5660eaef8472778add9880419ebd7
check prism https://github.com/prism-php/prism.git 5d6cc65b80b19cf3f22744703ac0c727b68cdca8

if [ "$FAIL" = "1" ]; then
    echo ""
    echo "Snapshots differ. If the change is intentional, run 'make eval-update'" >&2
    echo "and commit the refreshed files in eval/expected/." >&2
    exit 1
fi
