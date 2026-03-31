package web

import (
	"errors"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/social"
)

func TestPostValidationMessage(t *testing.T) {
	t.Parallel()

	if got := postValidationMessage(social.ErrPostContentTooLong); got != social.ErrPostContentTooLong.Error() {
		t.Fatalf("postValidationMessage() = %q, want %q", got, social.ErrPostContentTooLong.Error())
	}
	if got := postValidationMessage(errors.New("other")); got != "Invalid post content" {
		t.Fatalf("postValidationMessage() fallback = %q, want %q", got, "Invalid post content")
	}
}

func TestProfileValidationMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "missing display name", err: social.ErrDisplayNameMissing, want: social.ErrDisplayNameMissing.Error()},
		{name: "display name too long", err: social.ErrDisplayNameTooLong, want: social.ErrDisplayNameTooLong.Error()},
		{name: "bio too long", err: social.ErrBioTooLong, want: social.ErrBioTooLong.Error()},
		{name: "website too long", err: social.ErrWebsiteTooLong, want: social.ErrWebsiteTooLong.Error()},
		{name: "fallback", err: errors.New("other"), want: "Invalid profile fields"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := profileValidationMessage(tt.err); got != tt.want {
				t.Fatalf("profileValidationMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
