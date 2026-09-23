package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const MinPasswordLength = 8

func HashPassword(password string) (string, error) {
	if len([]byte(password)) < MinPasswordLength {
		return "", errors.New("password must be at least 8 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	// OWASP-recommended Argon2id baseline, encoded in a PHC-compatible form.
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=4$%s$%s", enc.EncodeToString(salt), enc.EncodeToString(hash)), nil
}

func CheckPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var salt, expected []byte
	var err error
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return false
	}
	if expected, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
