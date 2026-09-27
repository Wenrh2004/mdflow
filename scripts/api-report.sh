#!/usr/bin/env bash
# api-report compares every published module's API with its latest release tag
# using gorelease, and prints the report. Before v1 an incompatible change is
# allowed and only reported; from v1 on it fails, because it would need a new
# major version. Modules without a release tag yet are skipped.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/modules.sh

gorelease="go run golang.org/x/exp/cmd/gorelease@latest"
status=0
for dir in . $(published_modules); do
	prefix=""
	[ "$dir" != "." ] && prefix="$dir/"
	tag=$(git tag --list "${prefix}v*" --sort=-v:refname | grep -E "^${prefix}v[0-9]+\.[0-9]+\.[0-9]+$" | head -1 || true)
	if [ -z "$tag" ]; then
		echo "== $dir: no release tag yet; skipped"
		continue
	fi
	version=${tag#"$prefix"}
	echo "== $dir against $version"
	rc=0
	report=$(cd "$dir" && GOWORK=off $gorelease -base="$version" 2>&1) || rc=$?
	echo "$report"
	major=${version#v}
	major=${major%%.*}
	if grep -q "^## Incompatible changes" <<<"$report"; then
		if [ "$major" -ge 1 ]; then
			echo "$dir: incompatible API change against $version needs a new major version" >&2
			status=1
		fi
	elif [ "$rc" -ne 0 ]; then
		# gorelease itself failed (network, a missing base): never a silent pass
		# once compatibility is promised.
		echo "$dir: gorelease failed (exit $rc)" >&2
		[ "$major" -ge 1 ] && status=1
	fi
done
exit $status
