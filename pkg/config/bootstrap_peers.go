package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type bootstrapPeerManifest struct {
	Peers []struct {
		Address string `toml:"address"`
	} `toml:"peers"`
}

func loadBootstrapPeersFromFile() ([]string, error) {
	for _, candidate := range bootstrapPeerFileCandidates() {
		data, err := os.ReadFile(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read bootstrap peers file %s: %w", candidate, err)
		}

		var manifest bootstrapPeerManifest
		if err := toml.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("parse bootstrap peers file %s: %w", candidate, err)
		}

		peers := make([]string, 0, len(manifest.Peers))
		for _, peer := range manifest.Peers {
			if addr := strings.TrimSpace(peer.Address); addr != "" {
				peers = append(peers, addr)
			}
		}
		if len(peers) > 0 {
			return dedupeStrings(peers), nil
		}
	}

	return nil, nil
}

func bootstrapPeerFileCandidates() []string {
	candidates := []string{}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "configs", "bootstrap_peers.toml"))
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "configs", "bootstrap_peers.toml"),
			filepath.Join(exeDir, "..", "configs", "bootstrap_peers.toml"),
		)
	}

	return dedupeStrings(candidates)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
