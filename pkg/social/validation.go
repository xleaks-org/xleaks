package social

import (
	"errors"
	"unicode/utf8"
)

const (
	MaxPostContentChars = 5000
	MaxDisplayNameChars = 50
	MaxBioChars         = 500
	MaxWebsiteChars     = 200
)

var (
	ErrPostContentTooLong = errors.New("post content must not exceed 5000 characters")
	ErrDisplayNameMissing = errors.New("display_name is required")
	ErrDisplayNameTooLong = errors.New("display_name must not exceed 50 characters")
	ErrBioTooLong         = errors.New("bio must not exceed 500 characters")
	ErrWebsiteTooLong     = errors.New("website must not exceed 200 characters")
)

// ValidatePostContent enforces the protocol character limit for post bodies.
func ValidatePostContent(content string) error {
	if utf8.RuneCountInString(content) > MaxPostContentChars {
		return ErrPostContentTooLong
	}
	return nil
}

// ValidateProfileFields enforces the protocol limits for profile fields.
func ValidateProfileFields(displayName, bio, website string) error {
	if displayName == "" {
		return ErrDisplayNameMissing
	}
	if utf8.RuneCountInString(displayName) > MaxDisplayNameChars {
		return ErrDisplayNameTooLong
	}
	if utf8.RuneCountInString(bio) > MaxBioChars {
		return ErrBioTooLong
	}
	if utf8.RuneCountInString(website) > MaxWebsiteChars {
		return ErrWebsiteTooLong
	}
	return nil
}
