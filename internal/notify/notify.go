// Package notify delivers alert messages via Shoutrrr, whose single URL scheme
// covers Slack, Discord, Telegram, Gotify, ntfy, SMTP and many more providers.
package notify

import (
	"fmt"
	"strings"

	"github.com/containrrr/shoutrrr"
)

// Send delivers a message to a Shoutrrr URL. An empty URL is a no-op (alerts are
// optional). title, when set, is prefixed to the message body.
func Send(rawURL, title, message string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	body := message
	if title != "" {
		body = title + "\n" + message
	}
	if err := shoutrrr.Send(rawURL, body); err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	return nil
}

// Valid reports whether a Shoutrrr URL parses (used to validate before saving).
func Valid(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return true
	}
	_, err := shoutrrr.CreateSender(rawURL)
	return err == nil
}
