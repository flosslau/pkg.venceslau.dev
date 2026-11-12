# Copyright 2026 Bruno Venceslau
# SPDX-License-Identifier: Apache-2.0

# One target per gate. CI runs each gate as a job of the same name, and the
# ruleset of main requires those names. make check runs them all; make -k
# check goes on past a failing gate.

GATES := gofmt reuse vet test site lint

.PHONY: check $(GATES)
check: $(GATES)

# gofmt also runs go fix -diff, with and without the integration tag. Go 1.26
# prints the patch but exits 0, so the gate fails on any output; stderr is
# kept, so that a build error fails it too, and so does a "go: downloading"
# line when the toolchain changes. Files for other platforms or tags are not
# seen. The Go of mise.toml, which CI uses, is the reference: other versions
# know other fixes. TestGofmtGate pins this recipe.
gofmt:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }
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
