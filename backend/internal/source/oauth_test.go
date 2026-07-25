package source

import "testing"

// gmailIncrementalQuery turns the stored cursor into Gmail's `q` filter so a
// sync fetches only mail newer than the last run.
func TestGmailIncrementalQuery(t *testing.T) {
	cases := []struct {
		name, cursor, want string
	}{
		{"empty cursor fetches recent", "", ""},
		{"unparseable cursor is ignored", "not-a-time", ""},
		{"rfc3339 cursor becomes after: filter", "2026-07-23T03:39:26Z", "after:1784777966"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := gmailIncrementalQuery(tc.cursor); got != tc.want {
				t.Fatalf("gmailIncrementalQuery(%q) = %q, want %q", tc.cursor, got, tc.want)
			}
		})
	}
}
