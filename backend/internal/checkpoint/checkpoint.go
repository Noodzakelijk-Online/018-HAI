// Package checkpoint serializes and restores a small resume state so a
// long-running, phased task can survive a context loss or restart and continue
// where it left off. Pure JSON round-trip; the caller owns persistence.
package checkpoint

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Checkpoint captures where a phased task is.
type Checkpoint struct {
	Task      string   `json:"task"`
	Phase     string   `json:"phase"`
	Step      int      `json:"step"`
	Completed []string `json:"completed,omitempty"`
	Note      string   `json:"note,omitempty"`
}

// Encode serializes the checkpoint to JSON.
func (c Checkpoint) Encode() ([]byte, error) {
	if strings.TrimSpace(c.Task) == "" {
		return nil, fmt.Errorf("checkpoint task is required")
	}
	return json.Marshal(c)
}

// Decode parses a checkpoint from JSON.
func Decode(data []byte) (Checkpoint, error) {
	var c Checkpoint
	if err := json.Unmarshal(data, &c); err != nil {
		return Checkpoint{}, fmt.Errorf("invalid checkpoint: %w", err)
	}
	if strings.TrimSpace(c.Task) == "" {
		return Checkpoint{}, fmt.Errorf("checkpoint missing task")
	}
	return c, nil
}

// MarkComplete appends an item to the completed list if not already present and
// returns the updated checkpoint (value semantics — no mutation of the receiver).
func (c Checkpoint) MarkComplete(item string) Checkpoint {
	for _, done := range c.Completed {
		if done == item {
			return c
		}
	}
	out := c
	out.Completed = append(append([]string{}, c.Completed...), item)
	return out
}
