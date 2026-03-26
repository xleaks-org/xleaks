package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/xleaks-org/xleaks/pkg/api"
	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/storage"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

const defaultConfigPath = "~/.xleaks/config.toml"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(defaultConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	db, cas, err := setupDatabase(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	dataDir := cfg.DataDir()
	idHolder, kp := setupIdentity(dataDir, db)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p2pHost, err := setupP2P(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to set up P2P: %w", err)
	}
	if p2pHost != nil {
		defer p2pHost.Close()
	}

	svc := setupServices(ctx, db, cas, kp, idHolder)
	wireOutboundPublishers(p2pHost, svc)
	if err := backfillPinnedContent(db); err != nil {
		log.Printf("Warning: failed to backfill pinned content state: %v", err)
	}

	msgProcessor, messageHandler := wireP2PSubscriptions(ctx, p2pHost, kp, db, cas, svc)
	ensureTopicSubscription := newTopicSubscriber(p2pHost, messageHandler)
	wireContentExchange(p2pHost, cas)
	if msgProcessor != nil {
		msgProcessor.SetAutoFetchMedia(cfg.Media.AutoFetchMedia)
		if p2pHost != nil {
			if ce := p2pHost.ContentExchange(); ce != nil {
				msgProcessor.SetMediaFetcher(func(ctx context.Context, cidHex string) ([]byte, error) {
					return ce.FetchContent(ctx, cidHex, ce.FetchLocal)
				})
			}
		}
	}
	wireIndexerDiscovery(ctx, cfg, p2pHost, svc)

	identitySync := newIdentityRuntimeSyncer(p2pHost, messageHandler, ensureTopicSubscription, svc)
	identitySync(nil)

	// WU-1/WU-2: Start indexer mode if configured.
	idx := setupIndexer(ctx, db, dataDir, cfg, p2pHost)

	// WU-3: Feed incoming P2P posts/profiles into the indexer.
	if idx != nil && msgProcessor != nil {
		msgProcessor.SetIndexer(idx)
	}

	replicator := feed.NewReplicator(db, cas)
	maxStorage := int64(cfg.Node.MaxStorageGB) * 1024 * 1024 * 1024
	replicator.StartStorageManager(ctx, maxStorage, 5*time.Minute)

	cfgPath := defaultConfigPath
	webRoutes := setupWebHandler(db, idHolder, svc, cfg, p2pHost, dataDir, idx, identitySync, ensureTopicSubscription)
	deps := buildAPIDeps(db, cas, kp, idHolder, svc, p2pHost, cfg, cfgPath, webRoutes, identitySync, ensureTopicSubscription)

	server := api.NewServerWithConfig(api.ServerConfig{
		ListenAddr:      cfg.API.ListenAddress,
		EnableWebSocket: cfg.API.EnableWebSocket,
	}, deps)
	if wsHub := server.WSHub(); wsHub != nil {
		broadcast := func(eventType string, data interface{}) {
			wsHub.Broadcast(api.WSEvent{Type: eventType, Data: data})
		}
		svc.Notifs.SetBroadcaster(broadcast)
		if msgProcessor != nil {
			msgProcessor.SetBroadcaster(broadcast)
		}
	}
	return runServer(ctx, cancel, server, p2pHost, cfg)
}

// wireP2PSubscriptions sets up GossipSub topic subscriptions and feed hooks.
// It creates a MessageProcessor that handles incoming messages and returns it
// so callers can set optional fields (e.g. an indexer).
func wireP2PSubscriptions(
	ctx context.Context,
	host *p2p.Host,
	kp *identity.KeyPair,
	db *storage.DB,
	cas *content.ContentStore,
	svc *ServiceBundle,
) (*p2p.MessageProcessor, p2p.MessageHandler) {
	if host == nil {
		return nil, nil
	}

	mp := p2p.NewMessageProcessor(db, cas, svc.Notifs)
	var ensureTopic func(string) error
	var handler p2p.MessageHandler
	handler = func(ctx context.Context, _ p2p.PeerID, data []byte) {
		if err := mp.HandleMessage(ctx, data); err != nil {
			log.Printf("P2P message error: %v", err)
			return
		}
		if err := ensureObservedTopicSubscriptions(ensureTopic, data); err != nil {
			log.Printf("P2P topic tracking error: %v", err)
		}
	}
	ensureTopic = newTopicSubscriber(host, handler)

	if err := ensureTopic(p2p.GlobalTopic()); err != nil {
		log.Printf("Warning: failed to subscribe to %s: %v", p2p.GlobalTopic(), err)
	}
	if err := ensureTopic(p2p.ProfilesTopic()); err != nil {
		log.Printf("Warning: failed to subscribe to %s: %v", p2p.ProfilesTopic(), err)
	}
	if kp != nil && len(kp.PublicKeyBytes()) > 0 {
		topic := p2p.DMTopic(hex.EncodeToString(kp.PublicKeyBytes()))
		if err := ensureTopic(topic); err != nil {
			log.Printf("Warning: failed to subscribe to %s: %v", topic, err)
		}
	}

	svc.Feed.OnSubscribe = func(_ context.Context, pubkeyHex string) error {
		return ensureTopic(p2p.PostsTopic(pubkeyHex))
	}
	svc.Feed.OnUnsubscribe = func(pubkeyHex string) error {
		return host.Unsubscribe(p2p.PostsTopic(pubkeyHex))
	}
	if kp != nil && len(kp.PublicKeyBytes()) > 0 {
		if err := svc.Feed.ReloadSubscriptions(ctx, kp.PublicKeyBytes()); err != nil {
			log.Printf("Warning: failed to load subscriptions: %v", err)
		}
	}
	seedKnownFollowSubscriptions(db, ensureTopic)

	return mp, handler
}

// wireContentExchange configures the content exchange fetcher on the P2P host.
func wireContentExchange(host *p2p.Host, cas *content.ContentStore) {
	if host == nil {
		return
	}
	ce := host.ContentExchange()
	if ce == nil {
		return
	}
	ce.SetContentFetcher(func(cidHex string) ([]byte, error) {
		cidBytes, err := content.HexToCID(cidHex)
		if err != nil {
			return nil, err
		}
		return cas.Get(cidBytes)
	})
	ce.ServeContent(func(cidHex string) ([]byte, bool) {
		cidBytes, err := content.HexToCID(cidHex)
		if err != nil {
			return nil, false
		}
		data, err := cas.Get(cidBytes)
		if err != nil {
			return nil, false
		}
		return data, true
	})
}

// wireIndexerDiscovery adds known indexers and starts periodic DHT discovery.
func wireIndexerDiscovery(
	ctx context.Context,
	cfg *config.Config,
	host *p2p.Host,
	svc *ServiceBundle,
) {
	for _, url := range cfg.Indexer.KnownIndexers {
		svc.Indexer.AddIndexer(url)
	}
	if host == nil {
		return
	}
	go discoverIndexersPeriodically(ctx, host, svc)
}

func backfillPinnedContent(db *storage.DB) error {
	identities, err := db.GetIdentities()
	if err != nil {
		return err
	}
	for _, id := range identities {
		if err := db.SetPinnedForAuthor(id.Pubkey, true); err != nil {
			return err
		}
	}

	subs, err := db.GetSubscriptions(nil)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(subs))
	for _, sub := range subs {
		key := string(sub.Pubkey)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if pin, err := db.ShouldPinAuthor(sub.Pubkey); err != nil {
			return err
		} else if err := db.SetPinnedForAuthor(sub.Pubkey, pin); err != nil {
			return err
		}
	}
	return nil
}

// discoverIndexersPeriodically queries the DHT for indexers every 5 minutes.
func discoverIndexersPeriodically(ctx context.Context, host *p2p.Host, svc *ServiceBundle) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			indexers, err := host.FindIndexers(ctx)
			if err != nil {
				log.Printf("DHT indexer discovery failed: %v", err)
				continue
			}
			for _, info := range indexers {
				for _, addr := range info.Addrs {
					svc.Indexer.AddIndexer(fmt.Sprintf("http://%s", addr.String()))
				}
			}
			log.Printf("DHT indexer discovery: found %d indexer(s)", len(indexers))
		}
	}
}

func wireOutboundPublishers(host *p2p.Host, svc *ServiceBundle) {
	if svc == nil {
		return
	}

	svc.Posts.SetPublisher(host)
	svc.Reactions.SetPublisher(host)
	svc.Profiles.SetPublisher(host)
	svc.DMs.SetPublisher(host)
	svc.Follows.SetPublisher(host)
}

func newIdentityRuntimeSyncer(host *p2p.Host, handler p2p.MessageHandler, ensureTopic func(string) error, svc *ServiceBundle) func(*identity.KeyPair) {
	var mu sync.Mutex
	var currentDMTopic string

	return func(kp *identity.KeyPair) {
		mu.Lock()
		defer mu.Unlock()

		svc.Posts.SetIdentity(kp)
		svc.Reactions.SetIdentity(kp)
		svc.Profiles.SetIdentity(kp)
		svc.DMs.SetIdentity(kp)
		svc.Follows.SetIdentity(kp)
		if svc.Feed != nil {
			var ownerPubkey []byte
			if kp != nil {
				ownerPubkey = kp.PublicKeyBytes()
			}
			if err := svc.Feed.ReloadSubscriptions(context.Background(), ownerPubkey); err != nil {
				log.Printf("Warning: failed to reload subscriptions: %v", err)
			}
		}

		if host == nil || handler == nil {
			return
		}

		if currentDMTopic != "" {
			if err := host.Unsubscribe(currentDMTopic); err != nil && !strings.Contains(err.Error(), "not subscribed") {
				log.Printf("Warning: failed to unsubscribe from %s: %v", currentDMTopic, err)
			}
			currentDMTopic = ""
		}

		if kp == nil || len(kp.PublicKeyBytes()) == 0 {
			return
		}

		topic := p2p.DMTopic(hex.EncodeToString(kp.PublicKeyBytes()))
		if err := ensureTopic(topic); err != nil {
			log.Printf("Warning: failed to subscribe to %s: %v", topic, err)
			return
		}
		currentDMTopic = topic
	}
}

func newTopicSubscriber(host *p2p.Host, handler p2p.MessageHandler) func(string) error {
	if host == nil || handler == nil {
		return func(string) error { return nil }
	}
	return func(topic string) error {
		if topic == "" {
			return nil
		}
		return host.EnsureSubscribed(topic, handler)
	}
}

func seedKnownFollowSubscriptions(db *storage.DB, ensureTopic func(string) error) {
	if db == nil || ensureTopic == nil {
		return
	}
	profiles, err := db.GetAllProfiles()
	if err != nil {
		log.Printf("Warning: failed to load profiles for follow subscriptions: %v", err)
		return
	}
	for _, profile := range profiles {
		if len(profile.Pubkey) == 0 {
			continue
		}
		topic := p2p.FollowsTopic(hex.EncodeToString(profile.Pubkey))
		if err := ensureTopic(topic); err != nil {
			log.Printf("Warning: failed to subscribe to %s: %v", topic, err)
		}
	}
}

func ensureObservedTopicSubscriptions(ensureTopic func(string) error, data []byte) error {
	if ensureTopic == nil || len(data) == 0 {
		return nil
	}

	var env pb.Envelope
	if err := proto.Unmarshal(data, &env); err != nil {
		return err
	}

	switch payload := env.Payload.(type) {
	case *pb.Envelope_Post:
		if payload.Post == nil {
			return nil
		}
		if err := ensureTopic(p2p.FollowsTopic(hex.EncodeToString(payload.Post.Author))); err != nil {
			return err
		}
		if len(payload.Post.Id) > 0 {
			return ensureTopic(p2p.ReactionsTopic(hex.EncodeToString(payload.Post.Id)))
		}
	case *pb.Envelope_Profile:
		if payload.Profile == nil {
			return nil
		}
		return ensureTopic(p2p.FollowsTopic(hex.EncodeToString(payload.Profile.Author)))
	case *pb.Envelope_FollowEvent:
		if payload.FollowEvent == nil {
			return nil
		}
		return ensureTopic(p2p.FollowsTopic(hex.EncodeToString(payload.FollowEvent.Author)))
	}

	return nil
}
