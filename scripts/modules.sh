#!/usr/bin/env bash
# Shared helpers for the multi-module release tooling. Source it; do not run it.
#
# mdflow is published as a set of plugin modules: the dependency-free core at the
# repository root, one module per extension under extension/, and the all
# umbrella. Every published module is versioned in lockstep and tagged with its
# directory as the tag prefix (v0.1.0, extension/table/v0.1.0, all/v0.1.0), which
# is how the go command locates a nested module's version.
#
# bench is a development-only module. It is never tagged, so it alone may keep
# replace directives.

MODULE_PREFIX="github.com/Wenrh2004/mdflow"

# published_modules prints the directory of every published nested module,
# relative to the repository root. The root module (".") is not included.
published_modules() {
	find . -name go.mod -not -path './go.mod' -not -path './bench/*' -not -path '*/.*' \
		-exec dirname {} \; | sed 's|^\./||' | sort
}

# internal_requires prints the mdflow module paths a go.mod requires.
internal_requires() {
	go mod edit -json "$1" | python3 -c '
import json, sys
for r in json.load(sys.stdin).get("Require") or []:
    if r["Path"] == sys.argv[1] or r["Path"].startswith(sys.argv[1] + "/"):
        print(r["Path"], r["Version"])
' "$MODULE_PREFIX"
}
