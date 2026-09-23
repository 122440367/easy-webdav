package api

import (
	"errors"
	"path"
	"regexp"
	"strings"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func ValidateUsername(value string) error {
	if !usernamePattern.MatchString(value) || value == "." || value == ".." || strings.Contains(value, "..") {
		return errors.New("invalid username")
	}
	return nil
}

func ValidateRootDir(value string) (string, error) {
	if value == "" || value == "." || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) {
		return "", errors.New("root directory must be a non-empty relative path")
	}
	clean := path.Clean(strings.ReplaceAll(value, `\`, `/`))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "%2e") || strings.Contains(clean, "%2E") {
		return "", errors.New("root directory escapes storage root")
	}
	return clean, nil
}
