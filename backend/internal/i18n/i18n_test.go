package i18n

import "testing"

func TestTranslateEnglishAndDutch(t *testing.T) {
	if T(EN, "action.approve") != "Approve" {
		t.Fatalf("EN approve wrong")
	}
	if T(NL, "action.approve") != "Goedkeuren" {
		t.Fatalf("NL approve wrong")
	}
}

func TestFallbackToEnglishThenKey(t *testing.T) {
	// A key present only in EN falls back for NL.
	if T(NL, "nav.memory") == "" {
		t.Fatalf("NL should have nav.memory")
	}
	// A completely unknown key returns itself.
	if T(NL, "totally.unknown") != "totally.unknown" {
		t.Fatalf("unknown key should return the key")
	}
}

func TestNormalizeAndSupported(t *testing.T) {
	if Normalize("NL-nl") != NL || Normalize("en-US") != EN || Normalize("fr") != EN {
		t.Fatalf("normalize wrong")
	}
	if s := Supported(); len(s) != 2 || s[0] != EN || s[1] != NL {
		t.Fatalf("supported = %v", s)
	}
}
