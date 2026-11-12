# Copyright 2026 Bruno Venceslau
# SPDX-License-Identifier: Apache-2.0

# The checks CI runs, from one entry point: make check.

.PHONY: check
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }
	reuse lint
	go vet -tags=integration ./...
	go test -count=1 -race -tags=integration ./...
	go run .
