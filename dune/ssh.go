// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package dune

import (
	"errors"
	"strings"

	"github.com/apache/cloudstack-go/v2/cloudstack"
)

const (
	// TokenHeader is the WebSocket handshake header that carries the single-use token.
	TokenHeader = "X-CloudStack-Ssh-Token"
	// SSHPath is the default SSH WebSocket path.
	SSHPath = "/client/ssh"
)

// SSHEndpoint is a single-use credential for one SSH session.
// The token lasts about a minute and is consumed by the first WebSocket
// that presents it.
type SSHEndpoint struct {
	Token        string
	Path         string
	TokenHeader  string
	Expires      string
	WebSocketURL string
}

// SSHService opens an SSH session. The caller opens the WebSocket
// returned by CreateEndpoint.
type SSHService struct {
	cs     *cloudstack.CloudStackClient
	apiURL string
}

type sshEndpointWire struct {
	Token       string `json:"token"`
	Path        string `json:"path"`
	TokenHeader string `json:"tokenheader"`
	Expires     string `json:"expires"`
}

// CreateEndpoint requests an SSH session to instanceID.
// WebSocketURL uses the same host as the API URL. https becomes wss and http becomes ws.
// The token is not placed in that URL.
func (s *SSHService) CreateEndpoint(instanceID string) (*SSHEndpoint, error) {
	if instanceID == "" {
		return nil, errors.New("instance id is required")
	}
	custom, ok := s.cs.Custom.(*cloudstack.CustomService)
	if !ok || custom == nil {
		return nil, errors.New("SSH endpoint is unavailable")
	}
	params := &cloudstack.CustomServiceParams{}
	params.SetParam("virtualmachineid", instanceID)
	var nested struct {
		Endpoint sshEndpointWire `json:"sshendpoint"`
	}
	if err := custom.CustomRequest("createSshEndpoint", params, &nested); err != nil {
		return nil, err
	}
	wire := nested.Endpoint
	if wire.Token == "" || strings.ContainsAny(wire.Token, "\r\n") {
		return nil, errors.New("SSH endpoint returned an unusable token")
	}
	path := wire.Path
	if path == "" {
		path = SSHPath
	}
	header := wire.TokenHeader
	if header == "" {
		header = TokenHeader
	}
	if strings.ContainsAny(header, "\r\n") || strings.EqualFold(header, "cookie") {
		return nil, errors.New("SSH endpoint returned an unusable token header")
	}
	wsURL, err := WebSocketURL(s.apiURL, path)
	if err != nil {
		return nil, err
	}
	return &SSHEndpoint{
		Token:        wire.Token,
		Path:         path,
		TokenHeader:  header,
		Expires:      wire.Expires,
		WebSocketURL: wsURL,
	}, nil
}
