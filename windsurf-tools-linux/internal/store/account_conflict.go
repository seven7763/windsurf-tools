package store

import (
	"strings"
	"unicode"

	"windsurf-tools-linux/internal/models"
)

func AccountsConflict(existing models.Account, incoming models.Account) bool {
	if trimmedEqual(existing.RefreshToken, incoming.RefreshToken) {
		return true
	}
	if trimmedEqual(existing.WindsurfAPIKey, incoming.WindsurfAPIKey) {
		return true
	}
	if trimmedEqual(existing.Token, incoming.Token) {
		return true
	}
	if accountIdentityEmail(existing.Email) && accountIdentityEmail(incoming.Email) {
		return normalizeAccountEmail(existing.Email) == normalizeAccountEmail(incoming.Email)
	}
	return false
}

func trimmedEqual(a string, b string) bool {
	return strings.TrimSpace(a) != "" && strings.TrimSpace(a) == strings.TrimSpace(b)
}

func normalizeAccountEmail(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func accountIdentityEmail(email string) bool {
	clean := strings.TrimSpace(email)
	if clean == "" {
		return false
	}
	lower := strings.ToLower(clean)
	if strings.HasPrefix(lower, "jwt #") || strings.HasPrefix(lower, "key #") || strings.HasPrefix(lower, "token #") {
		return false
	}
	if strings.ContainsRune(clean, '@') {
		return true
	}
	if strings.HasPrefix(lower, "user_") {
		suffix := lower[len("user_"):]
		if suffix == "" {
			return false
		}
		for _, r := range suffix {
			if !unicode.IsLetter(r) && !unicode.IsNumber(r) {
				return false
			}
		}
		return true
	}
	return false
}
