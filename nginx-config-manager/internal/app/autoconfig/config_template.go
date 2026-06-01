package autoconfig

const configTemplate = `location /{{.URLPath}}/ {
	set $upstream_host {{.Host}};
    set $upstream_port {{.Port}};
    auth_request /auth-verify;
    proxy_pass http://$upstream_host:$upstream_port;
}
`
