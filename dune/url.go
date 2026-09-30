// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package dune

import (
	"fmt"
	"net/url"
	"strings"
)

// WebSocketURL turns an API URL into the SSH WebSocket URL.
// https becomes wss and http becomes ws. The API path and query are dropped.
// path is the WebSocket path, normally /client/ssh. A query in path is
// rejected so the token is not part of the URL.
func WebSocketURL(apiURL, path string) (string, error) {
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return "", fmt.Errorf("api URL: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", fmt.Errorf("API URL scheme is unsupported")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("API URL has no host")
	}
	if path == "" {
		path = SSHPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.ContainsAny(path, "?#") {
		return "", fmt.Errorf("ssh path must not carry a query")
	}
	parsed.Path = path
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.User = nil
	return parsed.String(), nil
}
