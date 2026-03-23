package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config represents the complete node configuration.
type Config struct {
	Node    NodeConfig    `toml:"node"`
	Network NetworkConfig `toml:"network"`
	API     APIConfig     `toml:"api"`
	Indexer IndexerConfig `toml:"indexer"`
	Media   MediaConfig   `toml:"media"`
	Identity IdentityConfig `toml:"identity"`
	Logging LoggingConfig `toml:"logging"`
}

type NodeConfig struct {
	DataDir      string `toml:"data_dir"`
	Mode         string `toml:"mode"`
	MaxStorageGB int    `toml:"max_storage_gb"`
}

type NetworkConfig struct {
	ListenAddresses    []string `toml:"listen_addresses"`
	BootstrapPeers     []string `toml:"bootstrap_peers"`
	RelayAddresses     []string `toml:"relay_addresses"`
	EnableRelay        bool     `toml:"enable_relay"`
	EnableMDNS         bool     `toml:"enable_mdns"`
	EnableHolePunching bool     `toml:"enable_hole_punching"`
	MaxPeers           int      `toml:"max_peers"`
	BandwidthLimitMbps int      `toml:"bandwidth_limit_mbps"`
}

type APIConfig struct {
	ListenAddress   string `toml:"listen_address"`
	EnableWebSocket bool   `toml:"enable_websocket"`
}

type IndexerConfig struct {
	PublicAPIAddress     string   `toml:"public_api_address"`
	MaxIndexedPublishers int      `toml:"max_indexed_publishers"`
	TrendingWindows      []string `toml:"trending_windows"`
	KnownIndexers        []string `toml:"known_indexers"`
}

type MediaConfig struct {
	MaxUploadSizeMB  int  `toml:"max_upload_size_mb"`
	AutoFetchMedia   bool `toml:"auto_fetch_media"`
	ThumbnailQuality int  `toml:"thumbnail_quality"`
}

type IdentityConfig struct {
	PassphraseMinLength int `toml:"passphrase_min_length"`
}

type LoggingConfig struct {
	Level      string `toml:"level"`
	File       string `toml:"file"`
	MaxSizeMB  int    `toml:"max_size_mb"`
	MaxBackups int    `toml:"max_backups"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Node: NodeConfig{
			DataDir:      "~/.xleaks",
			Mode:         "user",
			MaxStorageGB: 5,
		},
		Network: NetworkConfig{
			ListenAddresses: []string{"/ip4/0.0.0.0/tcp/7460", "/ip4/0.0.0.0/udp/7460/quic-v1"},
			BootstrapPeers: []string{
				"/dnsaddr/bootstrap1.xleaks.org/tcp/7460",
				"/dnsaddr/bootstrap2.xleaks.org/tcp/7460",
				"/dnsaddr/bootstrap3.xleaks.org/tcp/7460",
			},
			EnableRelay:        true,
			EnableMDNS:         true,
			EnableHolePunching: true,
			MaxPeers:           100,
			BandwidthLimitMbps: 0,
		},
		API: APIConfig{
			ListenAddress:   "127.0.0.1:7470",
			EnableWebSocket: true,
		},
		Indexer: IndexerConfig{
			PublicAPIAddress:     "0.0.0.0:7471",
			MaxIndexedPublishers: 100000,
			TrendingWindows:      []string{"1h", "6h", "24h", "7d"},
		},
		Media: MediaConfig{
			MaxUploadSizeMB:  100,
			AutoFetchMedia:   false,
			ThumbnailQuality: 80,
		},
		Identity: IdentityConfig{
			PassphraseMinLength: 8,
		},
		Logging: LoggingConfig{
			Level:      "info",
			File:       "~/.xleaks/logs/xleaks.log",
			MaxSizeMB:  50,
			MaxBackups: 3,
		},
	}
}

// Load reads a config file and merges with defaults.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	path = expandHome(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return cfg, nil
}

// Save writes the configuration to a TOML file.
func (c *Config) Save(path string) error {
	path = expandHome(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(c); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	return nil
}

// DataDir returns the expanded data directory path.
func (c *Config) DataDir() string {
	return expandHome(c.Node.DataDir)
}

// IsIndexer returns true if the node is running in indexer mode.
func (c *Config) IsIndexer() bool {
	return c.Node.Mode == "indexer"
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
