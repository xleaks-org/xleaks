package main

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/xleaks-org/xleaks/pkg/api/middleware"
)

const (
	apiTokenEnvVar     = "XLEAKS_API_TOKEN"
	apiTokenFileEnvVar = "XLEAKS_API_TOKEN_FILE"
)

func loadAPIToken() (string, error) {
	return loadAPITokenFromEnv(os.Getenv)
}

func loadAPITokenFromEnv(getenv func(string) string) (string, error) {
	token := strings.TrimSpace(getenv(apiTokenEnvVar))
	tokenFile := strings.TrimSpace(getenv(apiTokenFileEnvVar))

	if token != "" && tokenFile != "" {
		return "", fmt.Errorf("set only one of %s or %s", apiTokenEnvVar, apiTokenFileEnvVar)
	}
	if tokenFile != "" {
		loaded, err := middleware.LoadToken(tokenFile)
		if err != nil {
			return "", fmt.Errorf("load API token file %q: %w", tokenFile, err)
		}
		if loaded == "" {
			return "", fmt.Errorf("API token file %q is empty", tokenFile)
		}
		return loaded, nil
	}

	return token, nil
}

func validateAPIExposure(listenAddr, token string) error {
	if token != "" || isLoopbackListenAddress(listenAddr) {
		return nil
	}

	return fmt.Errorf(
		"refusing to start with API listen address %q without token auth; bind to loopback or set %s/%s",
		listenAddr,
		apiTokenEnvVar,
		apiTokenFileEnvVar,
	)
}

func isLoopbackListenAddress(listenAddr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(listenAddr))
	if err != nil {
		return false
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
