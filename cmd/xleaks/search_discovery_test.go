package main

import (
	"encoding/hex"
	"slices"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/p2p"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

func TestEnsureObservedTopicSubscriptions_ProfileSubscribesPublisherPostsForIndexer(t *testing.T) {
	t.Parallel()

	author := []byte{0x01, 0x02, 0x03}
	topics := make([]string, 0, 2)
	ensureTopic := func(topic string) error {
		topics = append(topics, topic)
		return nil
	}

	data, err := proto.Marshal(&pb.Envelope{
		Payload: &pb.Envelope_Profile{Profile: &pb.Profile{Author: author}},
	})
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}

	if err := ensureObservedTopicSubscriptions(ensureTopic, data, true); err != nil {
		t.Fatalf("ensureObservedTopicSubscriptions() error = %v", err)
	}

	authorHex := hex.EncodeToString(author)
	if !slices.Contains(topics, p2p.PostsTopic(authorHex)) {
		t.Fatalf("expected publisher posts topic in %v", topics)
	}
	if !slices.Contains(topics, p2p.FollowsTopic(authorHex)) {
		t.Fatalf("expected publisher follows topic in %v", topics)
	}
}

func TestEnsureObservedTopicSubscriptions_FollowEventExpandsTargetPostsForIndexer(t *testing.T) {
	t.Parallel()

	author := []byte{0x0a}
	target := []byte{0x0b}
	topics := make([]string, 0, 3)
	ensureTopic := func(topic string) error {
		topics = append(topics, topic)
		return nil
	}

	data, err := proto.Marshal(&pb.Envelope{
		Payload: &pb.Envelope_FollowEvent{FollowEvent: &pb.FollowEvent{
			Author: author,
			Target: target,
		}},
	})
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}

	if err := ensureObservedTopicSubscriptions(ensureTopic, data, true); err != nil {
		t.Fatalf("ensureObservedTopicSubscriptions() error = %v", err)
	}

	if !slices.Contains(topics, p2p.PostsTopic(hex.EncodeToString(author))) {
		t.Fatalf("expected follow author posts topic in %v", topics)
	}
	if !slices.Contains(topics, p2p.PostsTopic(hex.EncodeToString(target))) {
		t.Fatalf("expected follow target posts topic in %v", topics)
	}
}
