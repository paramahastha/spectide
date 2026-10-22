BIN := bin/spectide

.PHONY: fmt test build demo smoke
fmt:
	gofmt -w cmd internal

test:
	go test ./...

build:
	mkdir -p bin
	go build -o $(BIN) ./cmd/spectide

demo: build
	$(BIN) demo --project ./examples/fake-provider-demo

smoke: build
	$(BIN) doctor
	$(BIN) demo --project ./examples/fake-provider-demo
