# syntax=docker/dockerfile:1

# 1. Build the embedded frontend.
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build -- --outDir /src/dist

# 2. Build the static Go binary.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/dist ./internal/web/dist
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILT=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.built=${BUILT}" \
    -o /easy-webdav ./cmd/easy-webdav
# The data directory is created here so a fresh named/anonymous volume inherits
# the ownership of the unprivileged runtime user.
RUN mkdir -p /data/files && chown -R 1000:1000 /data

# 3. Ship the binary on distroless: no shell, no package manager, CA
#    certificates and tzdata included.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /easy-webdav /easy-webdav
COPY --from=build --chown=1000:1000 /data /data

ENV EW_DATA_DIR=/data \
    EW_STORAGE_DIR=/data/files \
    EW_LISTEN=0.0.0.0:8080
VOLUME ["/data"]
EXPOSE 8080
# 1000:1000 keeps the default non-root posture. To match a host account, run
# the container as root with -e EW_PUID/-e EW_PGID and the binary hands the data
# directory over before dropping privileges.
USER 1000:1000
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/easy-webdav", "healthcheck"]
ENTRYPOINT ["/easy-webdav"]
