BIN := bin/spectide
VERSION ?= dev

.PHONY: fmt format-check lint test web-test web-e2e web-build build demo smoke security verify load-test-sse release-prep
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

web-e2e:
	cd web && npm run test:e2e

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

verify: format-check lint test web-test web-e2e web-build build security smoke

load-test-sse:
	SPECTIDE_RUN_SSE_LOAD=1 go test ./internal/api -run TestSSEModerateLoadAcceptance -count=1 -timeout 3m -v

release-prep: web-build
	./scripts/prepare-release.sh "$(VERSION)"
