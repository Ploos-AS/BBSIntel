package cursor

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const separator = "\x00"

func Encode(parts ...string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, separator)))
}

func Decode(token string, wantParts int) ([]string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return nil, fmt.Errorf("decode cursor: %w", err)
	}
	parts := strings.Split(string(decoded), separator)
	if len(parts) != wantParts {
		return nil, fmt.Errorf("decode cursor: got %d parts, want %d", len(parts), wantParts)
	}
	return parts, nil
}
