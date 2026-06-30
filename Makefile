.PHONY: test test-int test-int-helixdb help

test:
	go test ./... -count=1

test-int-helixdb:
	RUN_HELIXDB_INTEGRATION=1 go test ./internal/helixdb/... -count=1 -run 'TestIntegration' -v

help:
	@grep -E '^[a-zA-Z_-]+:' Makefile | sort
