// Package ntypes
package ntypes

type SSENotificationRequest struct {
	Service          string            `json:"service"` // "email", "sms", "push"
	NotificationType string            `json:"notification_type"`
	UseTemplate      bool              `json:"use_template"`  // true or false
	Message          string            `json:"message"`       // The message to be sent
	TemplateID       string            `json:"template_id"`   // The ID of the template to be used if UseTemplate is true
	TemplateData     map[string]string `json:"template_data"` // Data to be used in the template
	Target           TagetSet          `json:"target_set"`
}

type TagetSet struct {
	IsAll      bool     `json:"is_all"`
	Categories []string `json:"categories"`
	ClientIDs  []string `json:"client_ids"`
}

type SSENotifyPayload struct{}

type NotifyACKPayload struct {
	Queue      string
	MsgID      string `json:"msg_id"`
	Akg        bool   `json:"akg"`   // true or false
	RetryCount int    `json:"retry"` // Number of retries
}
