package entities

import "testing"

func TestAutomationValidateRejectsUnsafeNginxFields(t *testing.T) {
	cases := []Automation{
		{Name: "Bad URL", URLPath: "../outside", Host: "backend", Port: 80},
		{Name: "Bad Old URL", URLPath: "safe", OldUrlPath: "../old", Host: "backend", Port: 80},
		{Name: "Bad Host", URLPath: "safe", Host: "backend;\nproxy_pass http://example.com;", Port: 80},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			if err := tc.Validate(); err == nil {
				t.Fatalf("expected validation error for %#v", tc)
			}
		})
	}
}

func TestAutomationValidateAllowsSafeServiceHost(t *testing.T) {
	auto := Automation{Name: "Safe", URLPath: "safe-route", OldUrlPath: "old-route", Host: "generic-auto", Port: 8080}
	if err := auto.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
