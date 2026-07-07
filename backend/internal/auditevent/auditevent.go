// Package auditevent builds structured, redaction-aware audit entries. Detail
// values attached under sensitive keys are automatically redacted so an audit
// trail never becomes a secret leak.
package auditevent

import (
	"strings"
	"time"
)

const redacted = "[redacted]"

// Event is a single immutable audit entry.
type Event struct {
	At       time.Time         `json:"at"`
	Actor    string            `json:"actor"`
	Action   string            `json:"action"`
	Resource string            `json:"resource"`
	Result   string            `json:"result"`
	Details  map[string]string `json:"details,omitempty"`
}

// New creates an audit event.
func New(at time.Time, actor, action, resource, result string) Event {
	return Event{At: at.UTC(), Actor: actor, Action: action, Resource: resource, Result: result}
}

// WithDetail attaches a detail, redacting the value when the key looks sensitive
// (password/token/secret/key/authorization). Returns the event for chaining.
func (e Event) WithDetail(key, value string) Event {
	if e.Details == nil {
		e.Details = map[string]string{}
	}
	if isSensitiveKey(key) {
		e.Details[key] = redacted
	} else {
		e.Details[key] = value
	}
	return e
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, needle := range []string{"password", "token", "secret", "apikey", "api_key", "authorization", "private"} {
		if strings.Contains(k, needle) {
			return true
		}
	}
	return false
}
