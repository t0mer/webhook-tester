package models

import "time"

// Token is a capture endpoint: a unique URL (and later email/DNS host) that
// records inbound requests and defines the default response returned to callers.
type Token struct {
	UUID               string     `json:"uuid"`
	Alias              string     `json:"alias,omitempty"`
	DefaultStatus      int        `json:"default_status"`
	DefaultContent     string     `json:"default_content"`
	DefaultContentType string     `json:"default_content_type"`
	Timeout            int        `json:"timeout"`       // drives the 100/timeout rpm rate limit (0 = unlimited)
	CORS               bool       `json:"cors"`          // emit permissive CORS headers
	Expiry             int        `json:"expiry"`        // ttl seconds after last activity (0 = never)
	Actions            bool       `json:"actions"`       // custom actions enabled
	RequestLimit       int        `json:"request_limit"` // max stored requests (0 = unlimited)
	Description        string     `json:"description"`
	Listen             int        `json:"listen"`   // CLI forwarding window (seconds)
	Redirect           string     `json:"redirect"` // redirect target URL
	Password           string     `json:"-"`        // basic-auth password, AES-GCM encrypted at rest
	GroupID            string     `json:"group_id,omitempty"`
	Premium            bool       `json:"premium"` // always true self-hosted
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	LatestRequestAt    *time.Time `json:"latest_request_at,omitempty"`
}
