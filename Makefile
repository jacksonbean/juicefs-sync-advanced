BINARY = juicefs-sync-advanced

.PHONY: build test clean vet run

build:
	go build -o $(BINARY) .

test:
	go test ./pkg/sync/... ./pkg/scan/...

vet:
	go vet ./cmd/... ./pkg/...

clean:
	rm -f $(BINARY)
	go clean

run:
	./$(BINARY)

.DEFAULT_GOAL := build
