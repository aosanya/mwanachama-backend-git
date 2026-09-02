.PHONY: build test test-pg vet lint clean

## Verify the module compiles cleanly.
build:
	go build ./...

## Unit tests (no DB required).
test:
	go test ./...

## Integration tests against a real Postgres instance.
## Expects PG* env vars (see .env.example once storage code lands).
test-pg:
	go test -tags=integration ./...

vet:
	go vet ./...

clean:
	rm -rf bin/
