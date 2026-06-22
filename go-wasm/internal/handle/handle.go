// Package handle normalizes and validates vanity @handles used as the public
// URL for a published page (mysite.net/@handle).
package handle

import "errors"

const max = 32

var (
	// ErrEmpty means nothing usable was left after normalization.
	ErrEmpty = errors.New("handle is empty")
	// ErrReserved means the handle collides with an app/route name.
	ErrReserved = errors.New("handle is reserved")
	// ErrInvalid means the handle has no alphanumeric character.
	ErrInvalid = errors.New("handle is invalid")
)

// reserved blocks handles that would shadow real routes or static assets, plus
// a few obvious squats.
var reserved = map[string]bool{
	"api": true, "assets": true, "created": true, "edit": true, "p": true,
	"public": true, "admin": true, "static": true, "app": true, "index": true,
	"favicon": true, "robots": true, "sitemap": true, "wasm_exec": true,
	"main": true, "styles": true, "health": true, "healthz": true,
	"about": true, "terms": true, "privacy": true, "login": true,
	"logout": true, "signup": true, "settings": true, "new": true,
}

// Normalize canonicalizes a raw handle: strips a leading '@', drops whitespace,
// lowercases, keeps only [a-z0-9._-], and caps length at 32. This is the
// authoritative form stored and used in URLs.
func Normalize(raw string) string {
	out := make([]rune, 0, len(raw))
	skippedAt := false
	for _, r := range raw {
		if !skippedAt && (r == '@') {
			skippedAt = true
			continue
		}
		switch {
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			out = append(out, r)
		default:
			// drop whitespace and any other character
		}
		if len(out) == max {
			break
		}
	}
	return string(out)
}

// Validate checks an already-normalized handle.
func Validate(h string) error {
	if h == "" {
		return ErrEmpty
	}
	if len(h) > max {
		return ErrInvalid
	}
	if reserved[h] {
		return ErrReserved
	}
	for _, r := range h {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return nil
		}
	}
	return ErrInvalid
}

// IsReserved reports whether a normalized handle is reserved.
func IsReserved(h string) bool { return reserved[h] }
