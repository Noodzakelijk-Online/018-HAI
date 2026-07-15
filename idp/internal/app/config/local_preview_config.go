package config

import "strings"

const (
	localLoginBypassEnabled = "LOCAL_LOGIN_BYPASS_ENABLED"
	firstRunAdminEmail      = "FIRST_RUN_ADMIN_EMAIL"
)

// localPreviewConfig is deliberately opt-in. It exists for a single-user,
// loopback-bound installation where the operator wants to open the dashboard
// without typing credentials during local development or demonstration.
type localPreviewConfig struct {
	Enabled    bool
	OwnerEmail string
}

func newLocalPreviewConfig() *localPreviewConfig {
	return &localPreviewConfig{
		Enabled:    getEnvBool(localLoginBypassEnabled, false),
		OwnerEmail: strings.TrimSpace(strings.ToLower(getEnvString(firstRunAdminEmail, ""))),
	}
}
