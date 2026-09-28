#!/usr/bin/env bash
# release pins every published module's internal requirements to VERSION,
# commits that change, and tags the root module and every nested module at the
# resulting commit. Tags are created locally; push them with --push or with the
# git push command this script prints.
#
#   scripts/release.sh v0.2.0          # prepare the commit and tags
#   scripts/release.sh v0.2.0 --push   # ...and push the commit and every tag
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/modules.sh

version="${1:-}"
push="${2:-}"
if ! [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
	echo "usage: scripts/release.sh vMAJOR.MINOR.PATCH [--push]" >&2
	exit 2
fi
if [ -n "$(git status --porcelain)" ]; then
	echo "working tree is not clean" >&2
	exit 1
fi

modules=$(published_modules)
for dir in $modules; do
	while read -r path _; do
		[ -n "$path" ] && go mod edit -require="$path@$version" "$dir/go.mod"
	done < <(internal_requires "$dir/go.mod")
done
# bench is not published, but its requirements should name the release too.
while read -r path _; do
	[ -n "$path" ] && go mod edit -require="$path@$version" bench/go.mod
done < <(internal_requires bench/go.mod)

scripts/check-modules.sh
make test

if [ -n "$(git status --porcelain)" ]; then
	git commit -am "release: $version"
fi

tags=("$version")
for dir in $modules; do
	tags+=("$dir/$version")
done
for tag in "${tags[@]}"; do
	git tag -a "$tag" -m "mdflow $tag"
done

branch=$(git rev-parse --abbrev-ref HEAD)
if [ "$push" = "--push" ]; then
	git push origin "$branch" "${tags[@]}"
else
	echo "tagged ${#tags[@]} modules at $(git rev-parse --short HEAD); publish with:"
	echo "  git push origin $branch ${tags[*]}"
fi
