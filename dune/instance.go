// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package dune

import "github.com/apache/cloudstack-go/v2/cloudstack"

// Instance is a virtual machine.
type Instance = cloudstack.VirtualMachine

type (
	DeployInstanceParams          = cloudstack.DeployVirtualMachineParams
	DeployInstanceResponse        = cloudstack.DeployVirtualMachineResponse
	DestroyInstanceParams         = cloudstack.DestroyVirtualMachineParams
	DestroyInstanceResponse       = cloudstack.DestroyVirtualMachineResponse
	ExpungeInstanceParams         = cloudstack.ExpungeVirtualMachineParams
	ExpungeInstanceResponse       = cloudstack.ExpungeVirtualMachineResponse
	StartInstanceParams           = cloudstack.StartVirtualMachineParams
	StartInstanceResponse         = cloudstack.StartVirtualMachineResponse
	StopInstanceParams            = cloudstack.StopVirtualMachineParams
	StopInstanceResponse          = cloudstack.StopVirtualMachineResponse
	UpdateInstanceParams          = cloudstack.UpdateVirtualMachineParams
	UpdateInstanceResponse        = cloudstack.UpdateVirtualMachineResponse
	ChangeInstanceServiceParams   = cloudstack.ChangeServiceForVirtualMachineParams
	ChangeInstanceServiceResponse = cloudstack.ChangeServiceForVirtualMachineResponse
	ListInstancesParams           = cloudstack.ListVirtualMachinesParams
)

// ListInstancesResponse is a page of instances.
type ListInstancesResponse struct {
	Count     int         `json:"count"`
	Instances []*Instance `json:"instances"`
}

// InstanceService manages instances.
type InstanceService struct {
	cs *cloudstack.CloudStackClient
}

func (s *InstanceService) NewDeployParams(serviceOfferingID, templateID, zoneID string) *DeployInstanceParams {
	return s.cs.VirtualMachine.NewDeployVirtualMachineParams(serviceOfferingID, templateID, zoneID)
}

func (s *InstanceService) Deploy(p *DeployInstanceParams) (*DeployInstanceResponse, error) {
	return s.cs.VirtualMachine.DeployVirtualMachine(p)
}

func (s *InstanceService) NewDestroyParams(id string) *DestroyInstanceParams {
	return s.cs.VirtualMachine.NewDestroyVirtualMachineParams(id)
}

func (s *InstanceService) Destroy(p *DestroyInstanceParams) (*DestroyInstanceResponse, error) {
	return s.cs.VirtualMachine.DestroyVirtualMachine(p)
}

func (s *InstanceService) NewExpungeParams(id string) *ExpungeInstanceParams {
	return s.cs.VirtualMachine.NewExpungeVirtualMachineParams(id)
}

func (s *InstanceService) Expunge(p *ExpungeInstanceParams) (*ExpungeInstanceResponse, error) {
	return s.cs.VirtualMachine.ExpungeVirtualMachine(p)
}

func (s *InstanceService) NewStartParams(id string) *StartInstanceParams {
	return s.cs.VirtualMachine.NewStartVirtualMachineParams(id)
}

func (s *InstanceService) Start(p *StartInstanceParams) (*StartInstanceResponse, error) {
	return s.cs.VirtualMachine.StartVirtualMachine(p)
}

func (s *InstanceService) NewStopParams(id string) *StopInstanceParams {
	return s.cs.VirtualMachine.NewStopVirtualMachineParams(id)
}

func (s *InstanceService) Stop(p *StopInstanceParams) (*StopInstanceResponse, error) {
	return s.cs.VirtualMachine.StopVirtualMachine(p)
}

func (s *InstanceService) NewUpdateParams(id string) *UpdateInstanceParams {
	return s.cs.VirtualMachine.NewUpdateVirtualMachineParams(id)
}

func (s *InstanceService) Update(p *UpdateInstanceParams) (*UpdateInstanceResponse, error) {
	return s.cs.VirtualMachine.UpdateVirtualMachine(p)
}

func (s *InstanceService) NewChangeServiceParams(id, serviceOfferingID string) *ChangeInstanceServiceParams {
	return s.cs.VirtualMachine.NewChangeServiceForVirtualMachineParams(id, serviceOfferingID)
}

func (s *InstanceService) ChangeService(p *ChangeInstanceServiceParams) (*ChangeInstanceServiceResponse, error) {
	return s.cs.VirtualMachine.ChangeServiceForVirtualMachine(p)
}

func (s *InstanceService) NewListParams() *ListInstancesParams {
	return s.cs.VirtualMachine.NewListVirtualMachinesParams()
}

func (s *InstanceService) List(p *ListInstancesParams) (*ListInstancesResponse, error) {
	resp, err := s.cs.VirtualMachine.ListVirtualMachines(p)
	if err != nil {
		return nil, err
	}
	instances := resp.VirtualMachines
	if instances == nil {
		instances = []*Instance{}
	}
	return &ListInstancesResponse{Count: resp.Count, Instances: instances}, nil
}
