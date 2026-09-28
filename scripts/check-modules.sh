#!/usr/bin/env bash
# check-modules verifies that every published module can be consumed outside
# this repository: no replace directives (the go command ignores them in a
# dependency, so a published replace is a module no one else can build), and
# every internal requirement pinned to one shared version.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/modules.sh

status=0
versions=""
for dir in $(published_modules); do
	gomod="$dir/go.mod"
	if go mod edit -json "$gomod" | python3 -c 'import json,sys; sys.exit(1 if json.load(sys.stdin).get("Replace") else 0)'; then :; else
		echo "$gomod: published module has replace directives; use go.work for local development" >&2
		status=1
	fi
	while read -r path version; do
		[ -z "$path" ] && continue
		versions="$versions$version"$'\n'
	done < <(internal_requires "$gomod")
done

distinct=$(printf '%s' "$versions" | sort -u | grep -c . || true)
if [ "$distinct" -gt 1 ]; then
	echo "internal requirements disagree on the mdflow version:" >&2
	printf '%s' "$versions" | sort | uniq -c >&2
	status=1
fi
exit $status
