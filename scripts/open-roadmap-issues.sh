#!/usr/bin/env bash
# open-roadmap-issues files every docs/specs/roadmap/P*.md as a GitHub issue.
# The first line (after "# ") is the title, the "Labels:" line the labels, and
# the rest the body. Requires an authenticated gh with permission to create
# issues and labels.
set -euo pipefail
cd "$(dirname "$0")/.."
for f in docs/specs/roadmap/P*.md; do
	title=$(head -1 "$f" | sed 's/^# //')
	labels=$(grep -m1 '^Labels:' "$f" | sed 's/^Labels: //')
	body=$(tail -n +4 "$f")
	IFS=', ' read -r -a list <<<"$labels"
	args=()
	for l in "${list[@]}"; do
		gh label create "$l" --force >/dev/null 2>&1 || true
		args+=(--label "$l")
	done
	gh issue create --title "$title" --body "$body" "${args[@]}"
done
