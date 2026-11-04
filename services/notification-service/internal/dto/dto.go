package dto

type ProcessNotificationInput struct {
	EventID       string                 `json:"event_id"`
	EventType     string                 `json:"event_type"`
	UserID        string                 `json:"user_id"`
	Recipient     string                 `json:"recipient"`
	ChannelCode   string                 `json:"channel_code"`
	Data          map[string]interface{} `json:"data"`
	CorrelationID string                 `json:"correlation_id"`
	TraceID       string                 `json:"trace_id"`
	Source        string                 `json:"source"`
}

