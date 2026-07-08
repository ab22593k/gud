.PHONY: test test-int test-int-helixdb help

test:
	go test ./... -count=1

test-int-mem:
	RUN_HELIXDB_INTEGRATION=1 go test ./internal/mem/... -count=1 -run 'TestIntegration' -v

help:
	@grep -E '^[a-zA-Z_-]+:' Makefile | sort
