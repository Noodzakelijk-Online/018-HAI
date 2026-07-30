package config

import (
	"errors"
	"net"
	"strings"
)

const (
	localLoginBypassEnabled = "LOCAL_LOGIN_BYPASS_ENABLED"
	firstRunAdminEmail      = "FIRST_RUN_ADMIN_EMAIL"
	gatewayHostBind         = "GATEWAY_HOST_BIND"
)

// localPreviewConfig is deliberately opt-in. It exists for a single-user,
// loopback-bound installation where the operator wants to open the dashboard
// without typing credentials during local development or demonstration.
type localPreviewConfig struct {
	Enabled         bool
	OwnerEmail      string
	GatewayHostBind string
}

func newLocalPreviewConfig() (*localPreviewConfig, error) {
	enabled := getEnvBool(localLoginBypassEnabled, false)
	hostBind := strings.TrimSpace(getEnvString(gatewayHostBind, ""))
	if enabled && !isLoopbackBind(hostBind) {
		return nil, errors.New("LOCAL_LOGIN_BYPASS_ENABLED requires GATEWAY_HOST_BIND to be an explicit loopback address")
	}

	return &localPreviewConfig{
		Enabled:         enabled,
		OwnerEmail:      strings.TrimSpace(strings.ToLower(getEnvString(firstRunAdminEmail, ""))),
		GatewayHostBind: hostBind,
	}, nil
}

func isLoopbackBind(hostBind string) bool {
	host := strings.Trim(strings.TrimSpace(hostBind), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
