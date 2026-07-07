// Package reminders computes which reminders are due at a given time. Pure and
// clock-injected so scheduling logic is deterministic and testable.
package reminders

import (
	"sort"
	"time"
)

// Reminder is a scheduled reminder.
type Reminder struct {
	ID    string    `json:"id"`
	DueAt time.Time `json:"dueAt"`
	Note  string    `json:"note,omitempty"`
}

// Due returns the reminders whose DueAt is at or before now, sorted by DueAt
// ascending (most overdue first).
func Due(reminders []Reminder, now time.Time) []Reminder {
	due := make([]Reminder, 0)
	for _, r := range reminders {
		if !r.DueAt.After(now) {
			due = append(due, r)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].DueAt.Before(due[j].DueAt) })
	return due
}

// NextDue returns the earliest upcoming reminder after now, or ok=false if none.
func NextDue(reminders []Reminder, now time.Time) (Reminder, bool) {
	var next Reminder
	found := false
	for _, r := range reminders {
		if r.DueAt.After(now) && (!found || r.DueAt.Before(next.DueAt)) {
			next = r
			found = true
		}
	}
	return next, found
}
