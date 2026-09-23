# Contributing

Use Conventional Commits such as `feat: add quota reporting` or `fix: reject escaped paths`.

Local development requires Go 1.25 and Node 22. Docker is optional; run `go test ./...`, `go vet ./...`, and `npm --prefix web run build` locally.

WebDAV protocol changes should preserve the upstream license and record local patches in `internal/webdav/xnet/UPSTREAM.md`.
