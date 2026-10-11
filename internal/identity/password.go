package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/argon2"
)

const passwordParameters = "$argon2id$v=19$m=19456,t=2,p=1$"
const dummyPassword = passwordParameters + "AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

var passwordSlots = make(chan struct{}, 2)
var ErrPasswordBusy = errors.New("password verification busy")
var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,63}$`)

func Username(raw string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(raw))
	if !usernamePattern.MatchString(n) {
		return "", fmt.Errorf("username must be 3–64 letters, numbers, dots, underscores or hyphens, starting with a letter")
	}
	return n, nil
}

func passwordKey(password string, salt []byte) ([]byte, error) {
	select {
	case passwordSlots <- struct{}{}:
		defer func() { <-passwordSlots }()
	default:
		return nil, ErrPasswordBusy
	}
	return argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32), nil
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", fmt.Errorf("password must contain 12–1024 bytes")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := passwordKey(password, salt)
	if err != nil {
		return "", err
	}
	return passwordParameters + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}

// Empty hashes use equivalent dummy work so unknown and disabled accounts have
// the same password-verification path. Stored parameter strings are bounded.
func VerifyPassword(hash, password string) (bool, error) {
	if len(password) > 1024 {
		return false, nil
	}
	missing := hash == ""
	if missing {
		hash = dummyPassword
	}
	if !strings.HasPrefix(hash, passwordParameters) {
		return false, nil
	}
	parts := strings.Split(strings.TrimPrefix(hash, passwordParameters), "$")
	if len(parts) != 2 || len(parts[0]) != 22 || len(parts[1]) != 43 {
		return false, nil
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return false, nil
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false, nil
	}
	got, err := passwordKey(password, salt)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1 && !missing, nil
}
