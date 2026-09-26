# mdflow is a multi-module workspace: the core module, an all-in-one umbrella,
# each extension under extension/, and the bench harness. These targets fan every
# check across every module so a single `make test` covers the whole repository.

# Every directory holding a go.mod, relative to the repo root.
MODULES := $(shell find . -name go.mod -not -path '*/.*' -exec dirname {} \;)

.PHONY: all test race lint fuzz work-sync tidy fmt vet build clean check-modules release

all: build vet test

# build compiles every module.
build:
	@set -e; for m in $(MODULES); do \
		echo "== build $$m =="; (cd $$m && go build ./...); \
	done

# vet runs go vet across every module.
vet:
	@set -e; for m in $(MODULES); do \
		echo "== vet $$m =="; (cd $$m && go vet ./...); \
	done

# test runs the unit tests of every module with the race detector and coverage.
test race:
	@set -e; for m in $(MODULES); do \
		echo "== test $$m =="; (cd $$m && go test -race -cover ./...); \
	done

# lint runs golangci-lint across every module.
lint:
	@set -e; for m in $(MODULES); do \
		echo "== lint $$m =="; (cd $$m && golangci-lint run ./...); \
	done

# fuzz runs every fuzz target in the core module for a short, fixed budget — long
# enough to catch a regression in CI, short enough not to stall it.
FUZZTIME ?= 20s
fuzz:
	@set -e; for m in . all; do \
		for t in $$(cd $$m && go test -list '^Fuzz' . | grep '^Fuzz'); do \
			echo "== $$m $$t =="; (cd $$m && go test -run '^$$' -fuzz "^$$t$$" -fuzztime=$(FUZZTIME) .); \
		done; \
	done

# work-sync reconciles go.work with the module set after adding a module.
work-sync:
	go work sync

# tidy runs go mod tidy in every module. Published modules require each other
# at the released version, so run it after that version is tagged; between
# releases go.work resolves the modules locally and `make work-sync` suffices.
tidy:
	@set -e; for m in $(MODULES); do \
		echo "== tidy $$m =="; (cd $$m && go mod tidy); \
	done

# fmt formats every module's sources.
fmt:
	gofmt -w $(shell find . -name '*.go' -not -path '*/.*')

# check-modules verifies every published module is consumable outside this
# repository: no replace directives, one shared internal version.
check-modules:
	scripts/check-modules.sh

# release tags every module in lockstep: make release VERSION=v0.2.0
release:
	scripts/release.sh $(VERSION)

clean:
	go clean ./...
