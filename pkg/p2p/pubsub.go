package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

// PeerID is an alias for libp2p peer.ID, exported for use by consumers.
type PeerID = peer.ID

// MessageHandler is a callback invoked when a message is received on a
// subscribed topic.
type MessageHandler func(ctx context.Context, from PeerID, data []byte)

// InitPubSub initializes GossipSub on this host. It must be called before
// any Subscribe or Publish operations.
func (h *Host) InitPubSub(ctx context.Context) error {
	ps, err := pubsub.NewGossipSub(ctx, h.host)
	if err != nil {
		return fmt.Errorf("creating GossipSub: %w", err)
	}
	h.pubsub = ps
	return nil
}

// Subscribe joins the given topic and begins reading messages. Each received
// message is dispatched to handler in a separate goroutine. To stop receiving
// messages, call Unsubscribe with the same topic name.
func (h *Host) Subscribe(topic string, handler MessageHandler) error {
	if h.pubsub == nil {
		return fmt.Errorf("pubsub not initialized; call InitPubSub first")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.topics[topic]; exists {
		return fmt.Errorf("already subscribed to topic %q", topic)
	}

	t, err := h.pubsub.Join(topic)
	if err != nil {
		return fmt.Errorf("joining topic %q: %w", topic, err)
	}

	sub, err := t.Subscribe()
	if err != nil {
		t.Close()
		return fmt.Errorf("subscribing to topic %q: %w", topic, err)
	}

	ctx, cancel := context.WithCancel(h.ctx)

	th := &topicHandle{
		topic:  t,
		sub:    sub,
		cancel: cancel,
	}
	h.topics[topic] = th

	// Read messages in the background.
	go h.readLoop(ctx, sub, handler)

	return nil
}

// EnsureSubscribed joins the given topic if needed and starts the read loop.
// It is safe to call repeatedly for the same topic.
func (h *Host) EnsureSubscribed(topic string, handler MessageHandler) error {
	h.mu.RLock()
	_, exists := h.topics[topic]
	h.mu.RUnlock()
	if exists {
		return nil
	}
	return h.Subscribe(topic, handler)
}

// FilteredMessageHandler is a callback invoked when a message passes rate
// limiting and replay protection checks. It receives the author's public key
// hex and the message type in addition to the raw data.
type FilteredMessageHandler func(ctx context.Context, from peer.ID, data []byte)

// SubscribeWithFilter joins the given topic like Subscribe, but applies
// rate limiting and replay protection before dispatching messages.
// The authorExtractor function extracts the author public key hex and
// message type ("post" or "reaction") from the raw message data. If it
// returns an error, the message is dropped.
func (h *Host) SubscribeWithFilter(
	topic string,
	handler MessageHandler,
	authorExtractor func(data []byte) (authorHex string, msgType string, cidHex string, err error),
) error {
	// Wrap the handler with rate limiting and replay protection.
	filtered := func(ctx context.Context, from peer.ID, data []byte) {
		authorHex, msgType, cidHex, err := authorExtractor(data)
		if err != nil {
			log.Printf("dropping message: failed to extract author info: %v", err)
			return
		}

		// If no CID was extracted, compute one from the message data.
		if cidHex == "" {
			hash := sha256.Sum256(data)
			cidHex = hex.EncodeToString(hash[:])
		}

		// Replay protection: drop if we've already seen this CID.
		if h.seenMessages != nil && h.seenMessages.CheckAndMark(cidHex) {
			return
		}

		// Rate limiting: drop if the author has exceeded their limit.
		if h.rateLimiter != nil && !h.rateLimiter.Allow(authorHex, msgType) {
			log.Printf("rate limit exceeded for author %s (type: %s)", authorHex, msgType)
			return
		}

		handler(ctx, from, data)
	}

	return h.Subscribe(topic, filtered)
}

// readLoop continuously reads messages from a subscription and dispatches
// them to the handler.
func (h *Host) readLoop(ctx context.Context, sub *pubsub.Subscription, handler MessageHandler) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			// Context cancelled or subscription closed; stop reading.
			return
		}

		// Skip messages from ourselves.
		if msg.ReceivedFrom == h.host.ID() {
			continue
		}

		handler(ctx, msg.ReceivedFrom, msg.Data)
	}
}

// computeMessageCID computes a SHA-256 hash of the message data and returns
// it as a hex string. This is used as a fallback CID for replay protection
// when the message doesn't contain an explicit CID.
func computeMessageCID(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// Unsubscribe cancels the subscription for the given topic and closes it.
func (h *Host) Unsubscribe(topic string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	th, exists := h.topics[topic]
	if !exists {
		return fmt.Errorf("not subscribed to topic %q", topic)
	}

	if th.cancel != nil {
		th.cancel()
	}
	if th.sub != nil {
		th.sub.Cancel()
	}
	if th.topic != nil {
		if err := th.topic.Close(); err != nil {
			delete(h.topics, topic)
			return fmt.Errorf("closing topic %q: %w", topic, err)
		}
	}

	delete(h.topics, topic)
	return nil
}

// Publish sends data to the given topic. If the topic has not been joined
// yet it is joined automatically (without subscribing).
func (h *Host) Publish(ctx context.Context, topic string, data []byte) error {
	if h.pubsub == nil {
		return fmt.Errorf("pubsub not initialized; call InitPubSub first")
	}

	h.mu.RLock()
	th, exists := h.topics[topic]
	h.mu.RUnlock()

	if exists {
		return th.topic.Publish(ctx, data)
	}

	// Join the topic for publishing only.
	t, err := h.pubsub.Join(topic)
	if err != nil {
		return fmt.Errorf("joining topic %q for publish: %w", topic, err)
	}

	h.mu.Lock()
	// Double-check in case another goroutine joined between RUnlock and Lock.
	if existing, ok := h.topics[topic]; ok {
		h.mu.Unlock()
		t.Close()
		return existing.topic.Publish(ctx, data)
	}
	h.topics[topic] = &topicHandle{topic: t}
	h.mu.Unlock()

	return t.Publish(ctx, data)
}

// ---------------------------------------------------------------------------
// Topic naming helpers
// ---------------------------------------------------------------------------

// PostsTopic returns the GossipSub topic for posts by a given author.
func PostsTopic(authorPubKeyHex string) string {
	return "/xleaks/posts/" + authorPubKeyHex
}

// ReactionsTopic returns the GossipSub topic for reactions to a given post.
func ReactionsTopic(postCIDHex string) string {
	return "/xleaks/reactions/" + postCIDHex
}

// ProfilesTopic returns the GossipSub topic for profile announcements.
func ProfilesTopic() string {
	return "/xleaks/profiles"
}

// FollowsTopic returns the GossipSub topic for follow events by a given author.
func FollowsTopic(authorPubKeyHex string) string {
	return "/xleaks/follows/" + authorPubKeyHex
}

// DMTopic returns the GossipSub topic for direct messages to a given recipient.
func DMTopic(recipientPubKeyHex string) string {
	return "/xleaks/dm/" + recipientPubKeyHex
}

// GlobalTopic returns the GossipSub topic for global announcements.
func GlobalTopic() string {
	return "/xleaks/global"
}
