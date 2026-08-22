package config

import "testing"

func TestValidatePortRejectsEphemeralAndOutOfRangePorts(t *testing.T) {
	for _, port := range []int{-1, 0, 65536} {
		if err := validatePort(port); err == nil {
			t.Fatalf("validatePort(%d) unexpectedly succeeded", port)
		}
	}
}

func TestValidatePortAcceptsUsablePorts(t *testing.T) {
	for _, port := range []int{1, 80, 65535} {
		if err := validatePort(port); err != nil {
			t.Fatalf("validatePort(%d): %v", port, err)
		}
	}
}
