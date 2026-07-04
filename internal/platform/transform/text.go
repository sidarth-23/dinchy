// Package transform provides small pure string normalization helpers.
package transform

import "strings"

func Email(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func Trim(value string) string {
	return strings.TrimSpace(value)
}
