package accountfeed

import "fmt"

// ItemType classifies a normalized feed item (§10.11).
type ItemType string

const (
	ItemEmail         ItemType = "email"
	ItemMessage       ItemType = "message"
	ItemDocument      ItemType = "document"
	ItemCalendarEvent ItemType = "calendar_event"
	ItemIssue         ItemType = "issue"
	ItemPullRequest   ItemType = "pull_request"
	ItemCard          ItemType = "card"
	ItemChat          ItemType = "chat"
	ItemFile          ItemType = "file"
)

func allItemTypes() []ItemType {
	return []ItemType{
		ItemEmail, ItemMessage, ItemDocument, ItemCalendarEvent, ItemIssue,
		ItemPullRequest, ItemCard, ItemChat, ItemFile,
	}
}

func (t ItemType) String() string { return string(t) }
func (t ItemType) IsValid() bool {
	for _, x := range allItemTypes() {
		if x == t {
			return true
		}
	}
	return false
}

// ParseItemType parses an item type string.
func ParseItemType(v string) (ItemType, error) {
	t := ItemType(v)
	if !t.IsValid() {
		return "", fmt.Errorf("accountfeed: invalid itemType %q", v)
	}
	return t, nil
}
