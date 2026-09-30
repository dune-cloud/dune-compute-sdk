// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package dune

import (
	"fmt"
	"strings"

	"github.com/apache/cloudstack-go/v2/cloudstack"
)

type Template = cloudstack.Template

type (
	RegisterTemplateParams   = cloudstack.RegisterTemplateParams
	RegisterTemplateResponse = cloudstack.RegisterTemplateResponse
	UpdateTemplateParams     = cloudstack.UpdateTemplateParams
	UpdateTemplateResponse   = cloudstack.UpdateTemplateResponse
	DeleteTemplateParams     = cloudstack.DeleteTemplateParams
	DeleteTemplateResponse   = cloudstack.DeleteTemplateResponse
	LinkUserDataParams       = cloudstack.LinkUserDataToTemplateParams
	LinkUserDataResponse     = cloudstack.LinkUserDataToTemplateResponse
	ListTemplatesParams      = cloudstack.ListTemplatesParams
	ListTemplatesResponse    = cloudstack.ListTemplatesResponse
)

// TemplateService manages templates.
type TemplateService struct {
	cs *cloudstack.CloudStackClient
}

func (s *TemplateService) NewRegisterParams(displayText, format, name, rawURL string) *RegisterTemplateParams {
	return s.cs.Template.NewRegisterTemplateParams(displayText, format, "KVM", name, rawURL)
}

func (s *TemplateService) Register(p *RegisterTemplateParams) (*RegisterTemplateResponse, error) {
	return s.cs.Template.RegisterTemplate(p)
}

func (s *TemplateService) NewUpdateParams(id string) *UpdateTemplateParams {
	return s.cs.Template.NewUpdateTemplateParams(id)
}

func (s *TemplateService) Update(p *UpdateTemplateParams) (*UpdateTemplateResponse, error) {
	return s.cs.Template.UpdateTemplate(p)
}

func (s *TemplateService) NewDeleteParams(id string) *DeleteTemplateParams {
	return s.cs.Template.NewDeleteTemplateParams(id)
}

func (s *TemplateService) Delete(p *DeleteTemplateParams) (*DeleteTemplateResponse, error) {
	return s.cs.Template.DeleteTemplate(p)
}

func (s *TemplateService) NewLinkUserDataParams() *LinkUserDataParams {
	return s.cs.Template.NewLinkUserDataToTemplateParams()
}

func (s *TemplateService) LinkUserData(p *LinkUserDataParams) (*LinkUserDataResponse, error) {
	return s.cs.Template.LinkUserDataToTemplate(p)
}

func (s *TemplateService) NewListParams(templateFilter string) *ListTemplatesParams {
	return s.cs.Template.NewListTemplatesParams(templateFilter)
}

func (s *TemplateService) List(p *ListTemplatesParams) (*ListTemplatesResponse, error) {
	if p != nil {
		if filter, ok := p.GetTemplatefilter(); ok && strings.EqualFold(strings.TrimSpace(filter), "all") {
			return nil, fmt.Errorf("template filter %q is not supported", "all")
		}
	}
	return s.cs.Template.ListTemplates(p)
}
