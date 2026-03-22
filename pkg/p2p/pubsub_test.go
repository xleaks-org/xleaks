package p2p

import "testing"

func TestTopicNaming(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{
			name:     "PostsTopic",
			got:      PostsTopic("abc123"),
			expected: "/xleaks/posts/abc123",
		},
		{
			name:     "ReactionsTopic",
			got:      ReactionsTopic("cid456"),
			expected: "/xleaks/reactions/cid456",
		},
		{
			name:     "ProfilesTopic",
			got:      ProfilesTopic(),
			expected: "/xleaks/profiles",
		},
		{
			name:     "FollowsTopic",
			got:      FollowsTopic("pub789"),
			expected: "/xleaks/follows/pub789",
		},
		{
			name:     "DMTopic",
			got:      DMTopic("recip000"),
			expected: "/xleaks/dm/recip000",
		},
		{
			name:     "GlobalTopic",
			got:      GlobalTopic(),
			expected: "/xleaks/global",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestPostsTopicContainsAuthor(t *testing.T) {
	author := "deadbeef"
	topic := PostsTopic(author)
	if topic != "/xleaks/posts/deadbeef" {
		t.Errorf("expected topic to contain author key, got %q", topic)
	}
}

func TestReactionsTopicContainsPostCID(t *testing.T) {
	cid := "0xfeedface"
	topic := ReactionsTopic(cid)
	if topic != "/xleaks/reactions/0xfeedface" {
		t.Errorf("expected topic to contain post CID, got %q", topic)
	}
}

func TestDMTopicContainsRecipient(t *testing.T) {
	recipient := "cafebabe"
	topic := DMTopic(recipient)
	if topic != "/xleaks/dm/cafebabe" {
		t.Errorf("expected topic to contain recipient key, got %q", topic)
	}
}

func TestFollowsTopicContainsAuthor(t *testing.T) {
	author := "f00bar"
	topic := FollowsTopic(author)
	if topic != "/xleaks/follows/f00bar" {
		t.Errorf("expected topic to contain author key, got %q", topic)
	}
}
