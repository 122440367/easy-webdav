FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
COPY internal/ /src/internal/
RUN npm run build

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /easy-webdav ./cmd/easy-webdav

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /easy-webdav /easy-webdav
VOLUME ["/data"]
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/easy-webdav"]
HEALTHCHECK --interval=30s --timeout=3s CMD ["/easy-webdav", "--version"]
