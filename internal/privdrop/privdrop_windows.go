//go:build windows

// Package privdrop is a no-op on Windows: there is no setuid model to follow
// and the service runs with the account configured by the installer.
package privdrop

// MaybeDrop always succeeds on Windows.
func MaybeDrop(string) error { return nil }
