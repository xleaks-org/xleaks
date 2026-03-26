package social

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/xleaks-org/xleaks/pkg/p2p"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// Publisher is the minimal interface needed to broadcast protobuf envelopes.
type Publisher interface {
	Publish(ctx context.Context, topic string, data []byte) error
}

func publishPost(ctx context.Context, publisher Publisher, post *pb.Post) error {
	if post == nil {
		return fmt.Errorf("post is nil")
	}
	return publishEnvelope(ctx, publisher, p2p.PostsTopic(hex.EncodeToString(post.Author)), &pb.Envelope{
		Payload: &pb.Envelope_Post{Post: post},
	})
}

func publishReaction(ctx context.Context, publisher Publisher, reaction *pb.Reaction) error {
	if reaction == nil {
		return fmt.Errorf("reaction is nil")
	}
	return publishEnvelope(ctx, publisher, p2p.ReactionsTopic(hex.EncodeToString(reaction.Target)), &pb.Envelope{
		Payload: &pb.Envelope_Reaction{Reaction: reaction},
	})
}

func publishProfile(ctx context.Context, publisher Publisher, profile *pb.Profile) error {
	if profile == nil {
		return fmt.Errorf("profile is nil")
	}
	return publishEnvelope(ctx, publisher, p2p.ProfilesTopic(), &pb.Envelope{
		Payload: &pb.Envelope_Profile{Profile: profile},
	})
}

func publishFollowEvent(ctx context.Context, publisher Publisher, event *pb.FollowEvent) error {
	if event == nil {
		return fmt.Errorf("follow event is nil")
	}
	return publishEnvelope(ctx, publisher, p2p.FollowsTopic(hex.EncodeToString(event.Author)), &pb.Envelope{
		Payload: &pb.Envelope_FollowEvent{FollowEvent: event},
	})
}

func publishDirectMessage(ctx context.Context, publisher Publisher, dm *pb.DirectMessage) error {
	if dm == nil {
		return fmt.Errorf("direct message is nil")
	}
	return publishEnvelope(ctx, publisher, p2p.DMTopic(hex.EncodeToString(dm.Recipient)), &pb.Envelope{
		Payload: &pb.Envelope_DirectMessage{DirectMessage: dm},
	})
}

// PublishMediaObject broadcasts media metadata on the global topic so peers can
// discover attachment descriptors before fetching the raw content by CID.
func PublishMediaObject(ctx context.Context, publisher Publisher, obj *pb.MediaObject) error {
	if obj == nil {
		return fmt.Errorf("media object is nil")
	}
	return publishEnvelope(ctx, publisher, p2p.GlobalTopic(), &pb.Envelope{
		Payload: &pb.Envelope_MediaObject{MediaObject: obj},
	})
}

func publishEnvelope(ctx context.Context, publisher Publisher, topic string, env *pb.Envelope) error {
	if publisher == nil {
		return nil
	}

	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	if err := publisher.Publish(ctx, topic, data); err != nil {
		return fmt.Errorf("publish envelope to %s: %w", topic, err)
	}
	return nil
}
