# Copyright 2026 Bruno Venceslau
# SPDX-License-Identifier: Apache-2.0

# One target per gate. CI runs each gate as a job of the same name, and the
# ruleset of main requires those names. make check runs them all; make -k
# check goes on past a failing gate.

GATES := gofmt reuse vet test site lint

.PHONY: check $(GATES)
check: $(GATES)

# gofmt reads every .go file, whatever its build tags, and its stderr is kept,
# so a file that does not parse fails the gate even when nothing builds it.
# The gate also runs go fix -diff, with and without the integration tag. Go
# 1.26 prints the patch but exits 0, so the gate fails on any output; stderr is
# kept, so that a build error fails it too, and so does a "go: downloading"
# line when the toolchain changes. go fix does not see files for other
# platforms or tags. The Go of mise.toml, which CI uses, is the reference:
# other versions know other fixes. TestGofmtGate pins this recipe.
gofmt:
	@fmt="$$(gofmt -l . 2>&1)"; \
		test -z "$$fmt" || { echo "gofmt needed on:"; printf '%s\n' "$$fmt"; exit 1; }
	@fix="$$(go fix -diff ./... 2>&1; go fix -diff -tags=integration ./... 2>&1)"; \
		test -z "$$fix" || { echo "go fix needed:"; printf '%s\n' "$$fix"; exit 1; }

reuse:
	reuse lint

vet:
	go vet -tags=integration ./...

test:
	go test -count=1 -race -tags=integration ./...

site:
	go run .

lint:
	golangci-lint run --build-tags=integration ./...
