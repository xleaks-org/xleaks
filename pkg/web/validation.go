package web

import (
	"errors"

	"github.com/xleaks-org/xleaks/pkg/social"
)

func postValidationMessage(err error) string {
	if errors.Is(err, social.ErrPostContentTooLong) {
		return social.ErrPostContentTooLong.Error()
	}
	return "Invalid post content"
}

func profileValidationMessage(err error) string {
	switch {
	case errors.Is(err, social.ErrDisplayNameMissing):
		return social.ErrDisplayNameMissing.Error()
	case errors.Is(err, social.ErrDisplayNameTooLong):
		return social.ErrDisplayNameTooLong.Error()
	case errors.Is(err, social.ErrBioTooLong):
		return social.ErrBioTooLong.Error()
	case errors.Is(err, social.ErrWebsiteTooLong):
		return social.ErrWebsiteTooLong.Error()
	default:
		return "Invalid profile fields"
	}
}
