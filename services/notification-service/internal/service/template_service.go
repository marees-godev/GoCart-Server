package service

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
)

type TemplateService interface {
	ResolveTemplate(ctx context.Context, eventType, channelID string) (*model.Template, error)
	Render(tmpl *model.Template, data map[string]interface{}) (subject, body string, err error)
}

type templateService struct {
	templateRepo repository.TemplateRepository
}

func NewTemplateService(templateRepo repository.TemplateRepository) TemplateService {
	return &templateService{
		templateRepo: templateRepo,
	}
}

func (s *templateService) ResolveTemplate(ctx context.Context, eventType, channelID string) (*model.Template, error) {
	if channelID != "" {
		tmpl, err := s.templateRepo.GetByEventTypeAndChannel(ctx, eventType, channelID)
		if err != nil {
			return nil, err
		}
		if tmpl != nil {
			return tmpl, nil
		}
	}

	tmpl, err := s.templateRepo.GetByEventType(ctx, eventType)
	if err != nil {
		return nil, err
	}
	if tmpl == nil {
		return nil, fmt.Errorf("no active template found for event_type: %s", eventType)
	}
	return tmpl, nil
}

func (s *templateService) Render(tmpl *model.Template, data map[string]interface{}) (string, string, error) {
	if tmpl == nil {
		return "", "", fmt.Errorf("template cannot be nil")
	}

	subjTmpl, err := template.New("subject").Option("missingkey=default").Parse(tmpl.SubjectTemplate)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse subject template: %w", err)
	}

	var subjBuf bytes.Buffer
	if err := subjTmpl.Execute(&subjBuf, data); err != nil {
		return "", "", fmt.Errorf("failed to render subject template: %w", err)
	}

	bodyTmpl, err := template.New("body").Option("missingkey=default").Parse(tmpl.BodyTemplate)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse body template: %w", err)
	}

	var bodyBuf bytes.Buffer
	if err := bodyTmpl.Execute(&bodyBuf, data); err != nil {
		return "", "", fmt.Errorf("failed to render body template: %w", err)
	}

	return subjBuf.String(), bodyBuf.String(), nil
}

