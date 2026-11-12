# Copyright 2026 Bruno Venceslau
# SPDX-License-Identifier: Apache-2.0

# One target per gate. CI runs each gate as a job of the same name, and the
# ruleset of main requires those names. make check runs them all; make -k
# check goes on past a failing gate.

GATES := gofmt reuse vet test site lint

.PHONY: check $(GATES)
check: $(GATES)

gofmt:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }

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
