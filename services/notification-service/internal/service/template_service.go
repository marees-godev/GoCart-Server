package service

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"text/template"

	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/templates"
)

type TemplateService interface {
	ResolveTemplate(ctx context.Context, eventType, channelID string) (*model.Template, error)
	Render(tmpl *model.Template, data map[string]interface{}) (subject, body string, err error)
}

type compiledTemplate struct {
	subjectTmpl *template.Template
	bodyTmpl    *template.Template
}

type templateService struct {
	templateRepo repository.TemplateRepository
	mu           sync.RWMutex
	cache        map[string]*compiledTemplate
}

func NewTemplateService(templateRepo ...repository.TemplateRepository) TemplateService {
	var repo repository.TemplateRepository
	if len(templateRepo) > 0 {
		repo = templateRepo[0]
	}
	return &templateService{
		templateRepo: repo,
		cache:        make(map[string]*compiledTemplate),
	}
}

func (s *templateService) ResolveTemplate(ctx context.Context, eventType, channelID string) (*model.Template, error) {
	if s.templateRepo != nil {
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
		if tmpl != nil {
			return tmpl, nil
		}
	}

	if fileTmpl, ok := templates.GetTemplate(eventType); ok {
		return fileTmpl, nil
	}

	return nil, fmt.Errorf("no active template found for event_type: %s", eventType)
}

func (s *templateService) getOrCompile(tmpl *model.Template) (*compiledTemplate, error) {
	cacheKey := tmpl.ID + ":" + tmpl.SubjectTemplate + ":" + tmpl.BodyTemplate
	s.mu.RLock()
	cached, ok := s.cache[cacheKey]
	s.mu.RUnlock()
	if ok {
		return cached, nil
	}

	subjTmpl, err := template.New("subject").Option("missingkey=default").Parse(tmpl.SubjectTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse subject template: %w", err)
	}

	bodyTmpl, err := template.New("body").Option("missingkey=default").Parse(tmpl.BodyTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse body template: %w", err)
	}

	compiled := &compiledTemplate{
		subjectTmpl: subjTmpl,
		bodyTmpl:    bodyTmpl,
	}

	s.mu.Lock()
	s.cache[cacheKey] = compiled
	s.mu.Unlock()

	return compiled, nil
}

func (s *templateService) Render(tmpl *model.Template, data map[string]interface{}) (string, string, error) {
	if tmpl == nil {
		return "", "", fmt.Errorf("template cannot be nil")
	}

	compiled, err := s.getOrCompile(tmpl)
	if err != nil {
		return "", "", err
	}

	var subjBuf bytes.Buffer
	if err := compiled.subjectTmpl.Execute(&subjBuf, data); err != nil {
		return "", "", fmt.Errorf("failed to render subject template: %w", err)
	}

	var bodyBuf bytes.Buffer
	if err := compiled.bodyTmpl.Execute(&bodyBuf, data); err != nil {
		return "", "", fmt.Errorf("failed to render body template: %w", err)
	}

	return subjBuf.String(), bodyBuf.String(), nil
}
