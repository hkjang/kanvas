# syntax=docker/dockerfile:1.7
FROM node:24-alpine AS web-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
COPY webembed/ /src/webembed/
RUN npm run build

FROM golang:1.26-alpine AS go-builder
ARG VERSION=0.3.0-dev
ARG COMMIT=unknown
ARG BUILT_AT=unknown
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /src/webembed/dist/ ./webembed/dist/
RUN GOMAXPROCS=2 CGO_ENABLED=0 GOOS=linux go build -p 2 -trimpath -ldflags="-s -w -X github.com/hkjang/kanvas/internal/buildinfo.Version=${VERSION} -X github.com/hkjang/kanvas/internal/buildinfo.Commit=${COMMIT} -X github.com/hkjang/kanvas/internal/buildinfo.BuiltAt=${BUILT_AT}" -o /out/kanvas ./cmd/kanvas

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && addgroup -S kanvas && adduser -S -G kanvas -h /var/lib/kanvas kanvas && mkdir -p /var/lib/kanvas && chown -R kanvas:kanvas /var/lib/kanvas
COPY --from=go-builder /out/kanvas /usr/local/bin/kanvas
USER kanvas
VOLUME ["/var/lib/kanvas"]
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/kanvas"]
