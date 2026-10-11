# The commands this repository is worked with, in one place.
#
# `gate` runs what .github/workflows/ci.yml runs, in the same order, so a green
# gate here means the same thing there. **CI stays the authority**: a check
# added to the workflow and not to this file is still enforced, and a target
# here that CI does not run is a convenience, not a gate.
#
# Everything a target produces goes to `.out/`, which is gitignored and can be
# deleted at any time - see the "Before you say it is done" section of AGENTS.md.
#
# What is deliberately not here: the CRD contract checks. They need a fresh
# clone of asgard-kube and `yq`, and running them from memory of a three-week-old
# checkout proves nothing. hack/README.md is that procedure and stays the
# procedure.

BIN := .out/asgard-cli
PKG := ./cmd/asgard-cli

# Where `install` puts the binary. Left empty, `go install` uses its own
# default, $(go env GOPATH)/bin - and an empty GOBIN in the environment means
# exactly that to go, so this stays out of the way until somebody sets it:
#
#     make install GOBIN=~/.local/bin
GOBIN ?=
export GOBIN

.DEFAULT_GOAL := help

.PHONY: help
help: ## what you can run
	@echo "make <target>"
	@echo
	@grep -hE '^[a-z][a-z-]*:.*## ' $(MAKEFILE_LIST) \
	  | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

.PHONY: build
build: ## build the binary into .out/
	@mkdir -p .out
	go build -o $(BIN) $(PKG)

# No ldflags, on purpose. `asgard-cli version` then falls back to the module and
# VCS metadata rather than claiming a version nobody released; only GoReleaser
# injects one, and .goreleaser.yaml is the only place that should.
.PHONY: install
install: ## install asgard-cli into GOBIN (default: `go env GOPATH`/bin)
	go install $(PKG)
	@dir="$${GOBIN:-$$(go env GOPATH)/bin}"; \
	 echo "installed $$dir/asgard-cli"; \
	 found=$$(command -v asgard-cli 2>/dev/null || true); \
	 if [ -n "$$found" ] && [ "$$found" != "$$dir/asgard-cli" ]; then \
	   echo "warning: PATH finds $$found first, so that is what runs."; \
	   echo "         remove it, or install there: make install GOBIN=$$(dirname $$found)"; \
	 fi

.PHONY: fmt
fmt: ## rewrite what gofmt would change
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## fail on anything gofmt would change
	@unformatted=$$(gofmt -l .); \
	 if [ -n "$$unformatted" ]; then \
	   echo "not gofmt'd:"; echo "$$unformatted"; exit 1; \
	 fi

.PHONY: vet
vet: ## go vet
	go vet ./...

.PHONY: test
test: ## go test
	go test ./...

# The corpus gates, and the CI job by the same name. `--paths` and
# check-doc-paths.py are one rule read from its two sides: the first fails on a
# landed document naming a file only this repository has, the second on one of
# our own documents naming a path that is gone.
#
# `--urls` is not here for the reason CI does not have it either: it fetches
# every docs.asgard-ai.com link, and a third party's outage is not this
# repository's build failure. Run `make audit-urls` when links are the subject.
.PHONY: audit
audit: build ## hold the material against itself
	$(BIN) audit-material --links
	$(BIN) audit-material --commands
	$(BIN) audit-material --bare
	$(BIN) audit-material --paths
	$(BIN) audit-material --unverified
	$(BIN) audit-material --sources
	go run ./hack doc-paths
	go run ./hack goal
	go run ./hack pass-list
	go run ./hack aliases
	go run ./hack write-path

.PHONY: audit-urls
audit-urls: build ## every external link still answers; needs the network
	$(BIN) audit-material --urls

.PHONY: gate
gate: fmt-check vet test audit ## everything CI checks

.PHONY: dotenv
dotenv: ## hold the Go and python .env implementations against each other
	go run ./hack/dotenv-agreement

# **The checks that need somebody else's repository.** They are the maintainer's
# rather than CI's, because asgard-core is private and a stale clone proves
# nothing; `go run ./hack sources` says when one is due, and `go run ./hack list`
# says what each needs.
.PHONY: audit-upstream
audit-upstream: ## hold the material against the clones; needs $ASGARD_KUBE, _DOCS, _CORE
	go run ./hack tables
	go run ./hack coverage
	go run ./hack counts
	go run ./hack processors
	go run ./hack validate-crs

.PHONY: snapshot
snapshot: ## build every platform through GoReleaser, publishing nothing
	@command -v goreleaser >/dev/null || { \
	   echo "goreleaser is not on PATH: brew install goreleaser"; exit 1; }
	goreleaser check
	goreleaser release --snapshot --clean --skip=publish

.PHONY: tidy
tidy: ## go mod tidy
	go mod tidy

.PHONY: clean
clean: ## delete everything a build wrote
	rm -rf .out dist
