BIN := bin/spectide

.PHONY: fmt lint test web-test web-build build demo smoke
fmt:
	gofmt -w cmd internal

lint:
	go vet ./...
	cd web && npm run typecheck

test:
	go test ./...

web-test:
	cd web && npm test

web-build:
	cd web && npm run build

build:
	mkdir -p bin
	go build -o $(BIN) ./cmd/spectide

demo: build
	$(BIN) demo --project ./examples/fake-provider-demo

smoke: build
	$(BIN) doctor
	$(BIN) demo --project ./examples/fake-provider-demo
