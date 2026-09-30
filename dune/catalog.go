// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package dune

import "github.com/apache/cloudstack-go/v2/cloudstack"

type (
	Network                      = cloudstack.Network
	ListNetworksParams           = cloudstack.ListNetworksParams
	ListNetworksResponse         = cloudstack.ListNetworksResponse
	Zone                         = cloudstack.Zone
	ListZonesParams              = cloudstack.ListZonesParams
	ListZonesResponse            = cloudstack.ListZonesResponse
	ServiceOffering              = cloudstack.ServiceOffering
	ListServiceOfferingsParams   = cloudstack.ListServiceOfferingsParams
	ListServiceOfferingsResponse = cloudstack.ListServiceOfferingsResponse
	DiskOffering                 = cloudstack.DiskOffering
	ListDiskOfferingsParams      = cloudstack.ListDiskOfferingsParams
	ListDiskOfferingsResponse    = cloudstack.ListDiskOfferingsResponse
	QueryJobParams               = cloudstack.QueryAsyncJobResultParams
	QueryJobResponse             = cloudstack.QueryAsyncJobResultResponse
	ListAPIsParams               = cloudstack.ListApisParams
	ListAPIsResponse             = cloudstack.ListApisResponse
	API                          = cloudstack.Api
)

type NetworkService struct{ cs *cloudstack.CloudStackClient }

func (s *NetworkService) NewListParams() *ListNetworksParams {
	return s.cs.Network.NewListNetworksParams()
}

func (s *NetworkService) List(p *ListNetworksParams) (*ListNetworksResponse, error) {
	return s.cs.Network.ListNetworks(p)
}

type ZoneService struct{ cs *cloudstack.CloudStackClient }

func (s *ZoneService) NewListParams() *ListZonesParams {
	return s.cs.Zone.NewListZonesParams()
}

func (s *ZoneService) List(p *ListZonesParams) (*ListZonesResponse, error) {
	return s.cs.Zone.ListZones(p)
}

type ServiceOfferingService struct{ cs *cloudstack.CloudStackClient }

func (s *ServiceOfferingService) NewListParams() *ListServiceOfferingsParams {
	return s.cs.ServiceOffering.NewListServiceOfferingsParams()
}

func (s *ServiceOfferingService) List(p *ListServiceOfferingsParams) (*ListServiceOfferingsResponse, error) {
	return s.cs.ServiceOffering.ListServiceOfferings(p)
}

type DiskOfferingService struct{ cs *cloudstack.CloudStackClient }

func (s *DiskOfferingService) NewListParams() *ListDiskOfferingsParams {
	return s.cs.DiskOffering.NewListDiskOfferingsParams()
}

func (s *DiskOfferingService) List(p *ListDiskOfferingsParams) (*ListDiskOfferingsResponse, error) {
	return s.cs.DiskOffering.ListDiskOfferings(p)
}

type JobService struct{ cs *cloudstack.CloudStackClient }

func (s *JobService) NewQueryParams(jobID string) *QueryJobParams {
	return s.cs.Asyncjob.NewQueryAsyncJobResultParams(jobID)
}

func (s *JobService) Query(p *QueryJobParams) (*QueryJobResponse, error) {
	return s.cs.Asyncjob.QueryAsyncJobResult(p)
}

type APIService struct{ cs *cloudstack.CloudStackClient }

func (s *APIService) NewListParams() *ListAPIsParams {
	return s.cs.APIDiscovery.NewListApisParams()
}

func (s *APIService) List(p *ListAPIsParams) (*ListAPIsResponse, error) {
	return s.cs.APIDiscovery.ListApis(p)
}

type (
	UserData             = cloudstack.UserData
	ListUserDataParams   = cloudstack.ListUserDataParams
	ListUserDataResponse = cloudstack.ListUserDataResponse
)

// UserDataService lists user data.
type UserDataService struct{ cs *cloudstack.CloudStackClient }

func (s *UserDataService) NewListParams() *ListUserDataParams {
	return s.cs.User.NewListUserDataParams()
}

func (s *UserDataService) List(p *ListUserDataParams) (*ListUserDataResponse, error) {
	return s.cs.User.ListUserData(p)
}
