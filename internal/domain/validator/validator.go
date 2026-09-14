package validator

import (
	"context"
	"strings"
	"unicode/utf8"
)

type Validator interface {
	Valid(context.Context) Evaluator
}

type Evaluator map[string]any

func (e *Evaluator) AddFieldError(key string, message any) {
	if *e == nil {
		*e = make(map[string]any)
	}

	if _, exists := (*e)[key]; !exists {
		(*e)[key] = message
	}
}

func (e *Evaluator) CheckField(ok bool, key, message string) {
	if !ok {
		e.AddFieldError(key, message)
	}
}

func NotBlank(value string) bool {
	return strings.TrimSpace(value) != ""
}

func MaxChars(value string, n int) bool {
	return utf8.RuneCountInString(value) <= n
}

func MinChars(value string, n int) bool {
	return utf8.RuneCountInString(value) >= n
}

// MatchesUUID validates the UUID format (v1 to v7) without allocating memory on the heap via regex.
func MatchesUUID(value string) bool {
	if len(value) != 36 {
		return false
	}

	for i := 0; i < len(value); i++ {
		c := value[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		case 14:
			// UUID version must be from '1' to '7'
			if c < '1' || c > '7' {
				return false
			}
		case 19:
			// UUID variant must be '8', '9', 'a', 'b', 'A', 'B'
			if c != '8' && c != '9' && c != 'a' && c != 'b' && c != 'A' && c != 'B' {
				return false
			}
		default:
			// Other positions must be hexadecimal [0-9a-fA-F]
			if !isHex(c) {
				return false
			}
		}
	}
	return true
}

// MatchesUUIDv7 validates the UUIDv7 format without allocating memory on the heap via regex.
func MatchesUUIDv7(value string) bool {
	if len(value) != 36 {
		return false
	}

	for i := 0; i < len(value); i++ {
		c := value[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		case 14:
			// UUID version must be strictly '7' for UUIDv7
			if c != '7' {
				return false
			}
		case 19:
			// UUID variant must be '8', '9', 'a', 'b', 'A', 'B'
			if c != '8' && c != '9' && c != 'a' && c != 'b' && c != 'A' && c != 'B' {
				return false
			}
		default:
			// Other positions must be hexadecimal [0-9a-fA-F]
			if !isHex(c) {
				return false
			}
		}
	}
	return true
}

// Helper to check if a character is a valid hexadecimal digit.
func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// MatchesNumber validates if the string contains only decimal digits without using regex.
func MatchesNumber(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
