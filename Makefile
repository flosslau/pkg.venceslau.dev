# The checks CI runs, from one entry point: make check.

.PHONY: check
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }
	go vet -tags=integration ./...
	go test -count=1 -race -tags=integration ./...
	go run .
