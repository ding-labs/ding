package identity

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DisplayName validates presentation text. Names never identify an account.
func DisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if !utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", fmt.Errorf("display name must contain 1–100 characters on one line")
	}
	return name, nil
}
