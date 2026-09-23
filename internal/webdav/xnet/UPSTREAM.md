# WebDAV upstream

Source: `golang.org/x/net/webdav`
Version: `v0.42.0`
License: BSD-3-Clause, see `LICENSE`

Local compatibility patches are tracked in this file and kept separate from
the upstream implementation where practical.

## Local patches

- MOVE treats an absent `Overwrite` header as `T`, per RFC 4918 section 9.9.
- Collection hrefs retain a trailing slash in PROPFIND responses.
- PROPPATCH reports unsupported properties with `403` propstats and returns
  `207 Multi-Status`.
