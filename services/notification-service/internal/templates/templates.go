package templates

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
)

//go:embed templates.json
var embeddedTemplatesJSON []byte

type FileTemplate struct {
	EventType       string `json:"event_type"`
	Channel         string `json:"channel"`
	SubjectTemplate string `json:"subject_template"`
	BodyTemplate    string `json:"body_template"`
}

var (
	loadOnce sync.Once
	registry map[string]*model.Template
)

func initRegistry() {
	registry = make(map[string]*model.Template)
	var list []FileTemplate
	if err := json.Unmarshal(embeddedTemplatesJSON, &list); err != nil {
		return
	}
	now := time.Now()
	for _, item := range list {
		tmpl := &model.Template{
			ID:              "",
			EventType:       item.EventType,
			SubjectTemplate: item.SubjectTemplate,
			BodyTemplate:    item.BodyTemplate,
			IsActive:        true,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		key := strings.ToLower(item.EventType)
		registry[key] = tmpl
		if item.Channel != "" {
			channelKey := strings.ToLower(item.EventType + ":" + item.Channel)
			registry[channelKey] = tmpl
		}
	}
}

// GetTemplate resolves a template from file storage by event_type and optional channel
func GetTemplate(eventType string, channelCode ...string) (*model.Template, bool) {
	loadOnce.Do(initRegistry)

	if len(channelCode) > 0 && channelCode[0] != "" {
		channelKey := strings.ToLower(eventType + ":" + channelCode[0])
		if tmpl, ok := registry[channelKey]; ok {
			return tmpl, true
		}
	}

	key := strings.ToLower(eventType)
	tmpl, ok := registry[key]
	return tmpl, ok
}

// GetAllTemplates returns all registered file-based templates
func GetAllTemplates() []*model.Template {
	loadOnce.Do(initRegistry)
	result := make([]*model.Template, 0, len(registry))
	seen := make(map[string]bool)
	for _, tmpl := range registry {
		if !seen[tmpl.EventType] {
			seen[tmpl.EventType] = true
			result = append(result, tmpl)
		}
	}
	return result
}

