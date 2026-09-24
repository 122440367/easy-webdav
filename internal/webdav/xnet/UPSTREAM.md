# WebDAV upstream

Source: `golang.org/x/net/webdav`
Version: `v0.42.0`
License: BSD-3-Clause, see `LICENSE`

Local compatibility patches are tracked in this file and kept separate from the
upstream implementation where practical. Each patch is an independent commit so
it can be replayed when the upstream snapshot is refreshed.

## Local patches

1. **MOVE/COPY treat an absent `Overwrite` header as `T`** (RFC 4918 section
   9.9). macOS Finder sends MOVE without the header and upstream answers `412`.
   Upstream issue: [golang/go#66059](https://github.com/golang/go/issues/66059).
   Patched in `webdav.go` (`handleCopyMove`): `Overwrite != "F"` instead of
   `Overwrite == "T"` for both COPY and MOVE.
   *Note: this change rode along with the baseline import commit.*

2. **PROPFIND collection hrefs keep their trailing slash.** Already covered by
   upstream v0.42.0 (`webdav.go` appends `/` for collection hrefs); recorded
   here because Windows interop (see issue
   [golang/go#23871](https://github.com/golang/go/issues/23871)) depends on it
   and we verify it in the integration tests. No local diff.

3. **PROPPATCH on a collection returns `207` with `403` propstats** instead of
   `500`. Disk-backed file systems cannot open a directory for writing (EISDIR
   on Unix, access denied on Windows) and the Windows Mini-Redirector
   PROPPATCHes collections routinely. Upstream context:
   [golang/go#23871](https://github.com/golang/go/issues/23871). Patched in
   `prop.go` (`patch`): EISDIR/permission failures while opening fall back to an
   all-forbidden propstat response.

## Local enhancements (not upstream bugs)

- `handlePut` in `webdav.go` first checks whether the FileSystem implements
  `Put(ctx, name, src) (os.FileInfo, bool, error)`; when it does, the whole body
  is handed to the file system so it can enforce quotas, write to a temporary
  file and rename atomically, and report `201`/`204` via the created flag.
  Error mapping: not-exist becomes `409`, permission becomes `403` and
  `ErrInsufficientStorage` becomes `507`.
- `copyFiles` in `file.go` removes the partially created destination when a
  nested copy trips the quota, and reports the file system's own status code
  (for example `507`) instead of a generic `500`.

## Vendored test adaptations

- `webdav_test.go` (`TestPrefix`) accepts either `301` or `307` for the
  redirect that `net/http`'s ServeMux issues when a collection URL is missing
  its trailing slash. Toolchains before Go 1.27 answer `301`, later ones answer
  `307` so that non-GET methods keep their verb (RFC 9110). The project builds
  and releases with Go 1.27, and this tolerance only keeps contributors on an
  older toolchain green.
