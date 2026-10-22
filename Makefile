BIN := bin/spectide

.PHONY: fmt format-check lint test web-test web-build build demo smoke security verify load-test-sse
fmt:
	gofmt -w cmd internal

format-check:
	@test -z "$$(gofmt -l cmd internal)"

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
	./scripts/demo.sh

smoke: build
	./scripts/smoke.sh

security:
	./scripts/check-secrets.sh

verify: format-check lint test web-test web-build build security smoke

load-test-sse:
	SPECTIDE_RUN_SSE_LOAD=1 go test ./internal/api -run TestSSEModerateLoadAcceptance -count=1 -timeout 3m -v
