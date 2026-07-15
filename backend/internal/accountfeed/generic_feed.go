package accountfeed

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	defaultMaxContentBytes  = 200_000
	defaultMaxMetadataBytes = 16_000
)

// GenericItem is one item in the generic feed response (§10.11).
type GenericItem struct {
	ExternalID   string         `json:"externalId"`
	ThreadID     string         `json:"threadId,omitempty"`
	Title        string         `json:"title"`
	Content      string         `json:"content"`
	SourceURI    string         `json:"sourceUri,omitempty"`
	ItemType     string         `json:"itemType"`
	Provider     string         `json:"provider"`
	AccountLabel string         `json:"accountLabel,omitempty"`
	ProjectKey   string         `json:"projectKey,omitempty"`
	ReceivedAt   *string        `json:"receivedAt,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	RawJSON      string         `json:"-"`
}

// GenericFeed is the generic feed response envelope (§10.11).
type GenericFeed struct {
	Cursor string        `json:"cursor,omitempty"`
	Items  []GenericItem `json:"items"`
}

// sourceURISecret flags secret-looking content that must not appear in a sourceUri.
var sourceURISecret = regexp.MustCompile(`(?i)(token|auth|api[_-]?key|secret|password|bearer)=`)

// Validate enforces the §10.11 item validation rules.
func (it GenericItem) Validate(maxContentBytes, maxMetadataBytes int) error {
	if strings.TrimSpace(it.ExternalID) == "" {
		return fmt.Errorf("accountfeed: item externalId required")
	}
	if strings.TrimSpace(it.Provider) == "" {
		return fmt.Errorf("accountfeed: item provider required")
	}
	if _, err := ParseProvider(it.Provider); err != nil {
		return err
	}
	if strings.TrimSpace(it.ItemType) == "" {
		return fmt.Errorf("accountfeed: item itemType required")
	}
	if _, err := ParseItemType(it.ItemType); err != nil {
		return err
	}
	if strings.TrimSpace(it.Title) == "" && strings.TrimSpace(it.Content) == "" {
		return fmt.Errorf("accountfeed: item %q requires title or content", it.ExternalID)
	}
	if maxContentBytes <= 0 {
		maxContentBytes = defaultMaxContentBytes
	}
	if len(it.Content) > maxContentBytes {
		return fmt.Errorf("accountfeed: item %q content exceeds %d bytes", it.ExternalID, maxContentBytes)
	}
	if sourceURISecret.MatchString(it.SourceURI) {
		return fmt.Errorf("accountfeed: item %q sourceUri must not contain secrets", it.ExternalID)
	}
	if maxMetadataBytes <= 0 {
		maxMetadataBytes = defaultMaxMetadataBytes
	}
	if it.Metadata != nil {
		if raw, err := json.Marshal(it.Metadata); err == nil && len(raw) > maxMetadataBytes {
			return fmt.Errorf("accountfeed: item %q metadata exceeds %d bytes", it.ExternalID, maxMetadataBytes)
		}
	}
	return nil
}

// ParseGenericFeed parses feed bytes as either the generic envelope
// {cursor, items:[...]} or a bare array [...]; each item is validated.
func ParseGenericFeed(data []byte, maxContentBytes, maxMetadataBytes int) (GenericFeed, error) {
	trimmed := strings.TrimSpace(string(data))
	var feed GenericFeed
	if strings.HasPrefix(trimmed, "{") {
		if err := decodeItems([]byte(trimmed), &feed, true); err != nil {
			return GenericFeed{}, err
		}
	} else if strings.HasPrefix(trimmed, "[") {
		if err := decodeItems([]byte(trimmed), &feed, false); err != nil {
			return GenericFeed{}, err
		}
	} else {
		return GenericFeed{}, fmt.Errorf("accountfeed: feed must be a JSON object or array")
	}
	for i := range feed.Items {
		if err := feed.Items[i].Validate(maxContentBytes, maxMetadataBytes); err != nil {
			return GenericFeed{}, fmt.Errorf("item %d: %w", i, err)
		}
	}
	return feed, nil
}

// decodeItems decodes into feed, preserving each item's exact raw JSON.
func decodeItems(data []byte, feed *GenericFeed, envelope bool) error {
	if envelope {
		var raw struct {
			Cursor string            `json:"cursor"`
			Items  []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("accountfeed: invalid feed envelope: %w", err)
		}
		feed.Cursor = raw.Cursor
		return appendRawItems(feed, raw.Items)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("accountfeed: invalid feed array: %w", err)
	}
	return appendRawItems(feed, items)
}

func appendRawItems(feed *GenericFeed, raws []json.RawMessage) error {
	for i, raw := range raws {
		var it GenericItem
		if err := json.Unmarshal(raw, &it); err != nil {
			return fmt.Errorf("accountfeed: item %d: %w", i, err)
		}
		it.RawJSON = string(raw)
		feed.Items = append(feed.Items, it)
	}
	return nil
}

// ToFeedItem converts a validated generic item to the normalized FeedItem used
// by ToOperationInput, deriving the operation type from the item type.
func (it GenericItem) ToFeedItem() FeedItem {
	body := it.Content
	return FeedItem{
		ExternalID:    it.ExternalID,
		Title:         firstNonEmpty(it.Title, it.Content),
		Body:          body,
		OperationType: operationTypeForItem(it.ItemType),
		Metadata:      it.Metadata,
		RawJSON:       it.RawJSON,
	}
}

// operationTypeForItem maps an item type to a default operation type.
func operationTypeForItem(itemType string) string {
	switch ItemType(itemType) {
	case ItemEmail, ItemMessage, ItemChat:
		return "review_message"
	case ItemIssue, ItemPullRequest:
		return "review_code_item"
	case ItemCard:
		return "review_task_card"
	case ItemCalendarEvent:
		return "review_calendar_event"
	case ItemDocument, ItemFile:
		return "review_document"
	default:
		return "review_source_item"
	}
}
