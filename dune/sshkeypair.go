// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package dune

import "github.com/apache/cloudstack-go/v2/cloudstack"

// SSHKeyPair is an SSH key pair. Register stores a public key; the private key stays with the caller.
type SSHKeyPair = cloudstack.SSHKeyPair

type (
	CreateSSHKeyPairParams     = cloudstack.CreateSSHKeyPairParams
	CreateSSHKeyPairResponse   = cloudstack.CreateSSHKeyPairResponse
	RegisterSSHKeyPairParams   = cloudstack.RegisterSSHKeyPairParams
	RegisterSSHKeyPairResponse = cloudstack.RegisterSSHKeyPairResponse
	DeleteSSHKeyPairParams     = cloudstack.DeleteSSHKeyPairParams
	DeleteSSHKeyPairResponse   = cloudstack.DeleteSSHKeyPairResponse
	ListSSHKeyPairsParams      = cloudstack.ListSSHKeyPairsParams
	ListSSHKeyPairsResponse    = cloudstack.ListSSHKeyPairsResponse
)

// SSHKeyPairService manages SSH key pairs.
type SSHKeyPairService struct {
	cs *cloudstack.CloudStackClient
}

func (s *SSHKeyPairService) NewCreateParams(name string) *CreateSSHKeyPairParams {
	return s.cs.SSH.NewCreateSSHKeyPairParams(name)
}

func (s *SSHKeyPairService) Create(p *CreateSSHKeyPairParams) (*CreateSSHKeyPairResponse, error) {
	return s.cs.SSH.CreateSSHKeyPair(p)
}

func (s *SSHKeyPairService) NewRegisterParams(name, publicKey string) *RegisterSSHKeyPairParams {
	return s.cs.SSH.NewRegisterSSHKeyPairParams(name, publicKey)
}

func (s *SSHKeyPairService) Register(p *RegisterSSHKeyPairParams) (*RegisterSSHKeyPairResponse, error) {
	return s.cs.SSH.RegisterSSHKeyPair(p)
}

func (s *SSHKeyPairService) NewDeleteParams(name string) *DeleteSSHKeyPairParams {
	return s.cs.SSH.NewDeleteSSHKeyPairParams(name)
}

func (s *SSHKeyPairService) Delete(p *DeleteSSHKeyPairParams) (*DeleteSSHKeyPairResponse, error) {
	return s.cs.SSH.DeleteSSHKeyPair(p)
}

func (s *SSHKeyPairService) NewListParams() *ListSSHKeyPairsParams {
	return s.cs.SSH.NewListSSHKeyPairsParams()
}

func (s *SSHKeyPairService) List(p *ListSSHKeyPairsParams) (*ListSSHKeyPairsResponse, error) {
	return s.cs.SSH.ListSSHKeyPairs(p)
}
