package models

import "time"

// Request types.
const (
	RequestTypeWeb   = "web"
	RequestTypeEmail = "email"
	RequestTypeDNS   = "dns"
)

// Request is a single captured inbound event (HTTP request, email, or DNS query).
type Request struct {
	UUID    string `json:"uuid"`
	TokenID string `json:"token_id"`
	Type    string `json:"type"` // web | email | dns
	Method  string `json:"method"`
	IP      string `json:"ip"`

	// Optional geo data, populated only when a GeoIP database is configured.
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	Region      string `json:"region,omitempty"`
	City        string `json:"city,omitempty"`

	Hostname  string              `json:"hostname"`
	UserAgent string              `json:"user_agent"`
	Content   string              `json:"content"`
	Query     map[string][]string `json:"query"`
	Headers   map[string][]string `json:"headers"`
	URL       string              `json:"url"`
	Size      int                 `json:"size"`
	Sorting   int64               `json:"sorting"` // unix millis, used for ordering & search

	CustomActionOutput map[string]any `json:"custom_action_output,omitempty"`
	CustomActionErrors map[string]any `json:"custom_action_errors,omitempty"`
	ExecTime           float64        `json:"time"` // action execution seconds

	Files     []File    `json:"files,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
