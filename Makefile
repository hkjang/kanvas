VERSION := $(shell tr -d '\n' < VERSION)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo local)
BUILT_AT := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
IMAGE := kanvas-v$(VERSION)

.PHONY: test web build image release-tar clean

test:
	go test ./...
	cd web && npm run build

web:
	cd web && npm ci && npm run build

build: web
	mkdir -p bin
	go build -trimpath -ldflags="-X github.com/hkjang/kanvas/internal/buildinfo.Version=v$(VERSION) -X github.com/hkjang/kanvas/internal/buildinfo.Commit=$(COMMIT) -X github.com/hkjang/kanvas/internal/buildinfo.BuiltAt=$(BUILT_AT)" -o bin/kanvas ./cmd/kanvas

image:
	docker build --build-arg VERSION=v$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILT_AT=$(BUILT_AT) -t $(IMAGE):latest .

release-tar: image
	mkdir -p dist
	docker save $(IMAGE):latest | gzip -9 > dist/$(IMAGE).tar.gz

clean:
	rm -f bin/kanvas dist/$(IMAGE).tar.gz
