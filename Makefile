.PHONY: build test lint web docker-build docker-test

build:
	go build -ldflags "-X main.version=dev" ./cmd/easy-webdav

test:
	go test ./...

lint:
	go vet ./...

web:
	npm --prefix web run build

docker-build:
	@docker version >/dev/null 2>&1 || (echo "Docker unavailable; skipping docker-build" && exit 0)
	docker build -f deploy/Dockerfile .

docker-test:
	@docker version >/dev/null 2>&1 || (echo "Docker unavailable; skipping docker-test" && exit 0)
	@echo "Docker compatibility tests are not configured yet"
