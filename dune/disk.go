// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package dune

import "github.com/apache/cloudstack-go/v2/cloudstack"

// Disk is a block volume.
type Disk = cloudstack.Volume

type (
	CreateDiskParams   = cloudstack.CreateVolumeParams
	CreateDiskResponse = cloudstack.CreateVolumeResponse
	DeleteDiskParams   = cloudstack.DeleteVolumeParams
	DeleteDiskResponse = cloudstack.DeleteVolumeResponse
	AttachDiskParams   = cloudstack.AttachVolumeParams
	AttachDiskResponse = cloudstack.AttachVolumeResponse
	DetachDiskParams   = cloudstack.DetachVolumeParams
	DetachDiskResponse = cloudstack.DetachVolumeResponse
	ResizeDiskParams   = cloudstack.ResizeVolumeParams
	ResizeDiskResponse = cloudstack.ResizeVolumeResponse
	UpdateDiskParams   = cloudstack.UpdateVolumeParams
	UpdateDiskResponse = cloudstack.UpdateVolumeResponse
	ListDisksParams    = cloudstack.ListVolumesParams
)

// ListDisksResponse is a page of disks.
type ListDisksResponse struct {
	Count int     `json:"count"`
	Disks []*Disk `json:"disks"`
}

// DiskService manages disks.
type DiskService struct {
	cs *cloudstack.CloudStackClient
}

func (s *DiskService) NewCreateParams() *CreateDiskParams {
	return s.cs.Volume.NewCreateVolumeParams()
}

func (s *DiskService) Create(p *CreateDiskParams) (*CreateDiskResponse, error) {
	return s.cs.Volume.CreateVolume(p)
}

func (s *DiskService) NewDeleteParams(id string) *DeleteDiskParams {
	return s.cs.Volume.NewDeleteVolumeParams(id)
}

func (s *DiskService) Delete(p *DeleteDiskParams) (*DeleteDiskResponse, error) {
	return s.cs.Volume.DeleteVolume(p)
}

func (s *DiskService) NewAttachParams(id, instanceID string) *AttachDiskParams {
	return s.cs.Volume.NewAttachVolumeParams(id, instanceID)
}

func (s *DiskService) Attach(p *AttachDiskParams) (*AttachDiskResponse, error) {
	return s.cs.Volume.AttachVolume(p)
}

func (s *DiskService) NewDetachParams() *DetachDiskParams {
	return s.cs.Volume.NewDetachVolumeParams()
}

func (s *DiskService) Detach(p *DetachDiskParams) (*DetachDiskResponse, error) {
	return s.cs.Volume.DetachVolume(p)
}

func (s *DiskService) NewResizeParams(id string) *ResizeDiskParams {
	return s.cs.Volume.NewResizeVolumeParams(id)
}

func (s *DiskService) Resize(p *ResizeDiskParams) (*ResizeDiskResponse, error) {
	return s.cs.Volume.ResizeVolume(p)
}

func (s *DiskService) NewUpdateParams() *UpdateDiskParams {
	return s.cs.Volume.NewUpdateVolumeParams()
}

func (s *DiskService) Update(p *UpdateDiskParams) (*UpdateDiskResponse, error) {
	return s.cs.Volume.UpdateVolume(p)
}

func (s *DiskService) NewListParams() *ListDisksParams {
	return s.cs.Volume.NewListVolumesParams()
}

func (s *DiskService) List(p *ListDisksParams) (*ListDisksResponse, error) {
	resp, err := s.cs.Volume.ListVolumes(p)
	if err != nil {
		return nil, err
	}
	disks := resp.Volumes
	if disks == nil {
		disks = []*Disk{}
	}
	return &ListDisksResponse{Count: resp.Count, Disks: disks}, nil
}
