// Package analytics provides local-first, privacy-preserving usage aggregation.
// It computes counts entirely in-process from events the caller already holds —
// no external analytics service, no per-user tracking beyond what is passed in.
package analytics

import (
	"sort"
	"time"
)

// Event is a single recorded usage event.
type Event struct {
	Type string    `json:"type"`
	At   time.Time `json:"at"`
}

// TypeCount is an aggregated count for one event type.
type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// Summary is the aggregated result.
type Summary struct {
	Total      int         `json:"total"`
	ByType     []TypeCount `json:"byType"`
	ByDay      []TypeCount `json:"byDay"` // Type holds the YYYY-MM-DD day
	DistinctT  int         `json:"distinctTypes"`
	FirstEvent *time.Time  `json:"firstEvent,omitempty"`
	LastEvent  *time.Time  `json:"lastEvent,omitempty"`
}

// Aggregate summarizes events by type and by UTC day. Output slices are sorted
// (types by count desc then name; days ascending) for stable rendering.
func Aggregate(events []Event) Summary {
	typeCounts := map[string]int{}
	dayCounts := map[string]int{}
	var first, last time.Time

	for _, e := range events {
		if e.Type == "" {
			continue
		}
		typeCounts[e.Type]++
		day := e.At.UTC().Format("2006-01-02")
		dayCounts[day]++
		if first.IsZero() || e.At.Before(first) {
			first = e.At
		}
		if last.IsZero() || e.At.After(last) {
			last = e.At
		}
	}

	summary := Summary{DistinctT: len(typeCounts)}
	for typ, count := range typeCounts {
		summary.ByType = append(summary.ByType, TypeCount{Type: typ, Count: count})
		summary.Total += count
	}
	sort.Slice(summary.ByType, func(i, j int) bool {
		if summary.ByType[i].Count != summary.ByType[j].Count {
			return summary.ByType[i].Count > summary.ByType[j].Count
		}
		return summary.ByType[i].Type < summary.ByType[j].Type
	})
	for day, count := range dayCounts {
		summary.ByDay = append(summary.ByDay, TypeCount{Type: day, Count: count})
	}
	sort.Slice(summary.ByDay, func(i, j int) bool { return summary.ByDay[i].Type < summary.ByDay[j].Type })

	if !first.IsZero() {
		f := first.UTC()
		l := last.UTC()
		summary.FirstEvent = &f
		summary.LastEvent = &l
	}
	return summary
}
