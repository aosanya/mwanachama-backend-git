.PHONY: build test test-pg vet lint clean

## Verify the module compiles cleanly.
build:
	go build ./...

## Unit tests (no DB required).
test:
	go test ./...

## Integration tests against a real Postgres instance.
## Expects POSTGRES_URL (see .env.example; scripts/local-postgres.sh stands
## one up quickly). Applies this domain's DDL itself on each run.
test-pg:
	go test -tags=integration ./...

vet:
	go vet ./...

clean:
	rm -rf bin/
