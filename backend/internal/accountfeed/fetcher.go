package accountfeed

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"automation-hub-backend/internal/pathsafety"
)

const maxFeedBytes = 5 << 20 // 5 MiB bound on a fetched feed

// FetchOptions configure feed fetching.
type FetchOptions struct {
	FeedsRoot string // allowlisted root for local file feeds
	AllowHTTP bool   // HTTP feeds only fetched when enabled
}

// fetchFeedBytes reads the raw feed bytes for a feed, confining local files to
// the feeds root and validating HTTP URLs.
func fetchFeedBytes(ctx context.Context, feed Feed, opts FetchOptions) ([]byte, error) {
	switch feed.SourceType {
	case SourceLocalJSONFile:
		if !pathsafety.IsSafeRelative(feed.Path) {
			return nil, fmt.Errorf("accountfeed: unsafe feed path %q", feed.Path)
		}
		full, err := pathsafety.SafeJoin(opts.FeedsRoot, feed.Path)
		if err != nil {
			return nil, fmt.Errorf("accountfeed: %w", err)
		}
		return readBoundedLocalFeedFile(ctx, full)
	case SourceHTTPJSONFeed:
		if !opts.AllowHTTP {
			return nil, fmt.Errorf("accountfeed: HTTP feeds are disabled (set the enable flag to allow %s)", feed.URL)
		}
		if err := validateFeedURL(feed.URL); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
		if err != nil {
			return nil, err
		}
		client := &http.Client{
			Timeout:   10 * time.Second,
			Transport: accountFeedHTTPTransport(),
			// Each redirect is a new destination that has not passed the feed URL
			// validation and network boundary checks. Return it to the caller instead.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("accountfeed: feed fetch failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("accountfeed: feed HTTP %d", resp.StatusCode)
		}
		return readBoundedFeedBytes(ctx, resp.Body)
	default:
		return nil, fmt.Errorf("accountfeed: unsupported sourceType %q", feed.SourceType)
	}
}

func readBoundedLocalFeedFile(ctx context.Context, full string) ([]byte, error) {
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxFeedBytes {
		return nil, fmt.Errorf("accountfeed: feed exceeds maximum size of %d bytes", maxFeedBytes)
	}
	file, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readBoundedFeedBytes(ctx, file)
}

func readBoundedFeedBytes(ctx context.Context, reader io.Reader) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxFeedBytes+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(data) > maxFeedBytes {
		return nil, fmt.Errorf("accountfeed: feed exceeds maximum size of %d bytes", maxFeedBytes)
	}
	return data, nil
}

// validateFeedURL rejects private, link-local, metadata, and unspecified hosts.
// Localhost is allowed only for local development (§10.11). Host names are
// resolved and checked again immediately before dialing in accountFeedHTTPTransport.
func validateFeedURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("accountfeed: invalid feed URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("accountfeed: feed URL scheme must be http/https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("accountfeed: feed URL host is empty")
	}
	if strings.Contains(strings.ToLower(host), "metadata") {
		return fmt.Errorf("accountfeed: metadata host not allowed")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && accountFeedAddressBlocked(ip) {
		return fmt.Errorf("accountfeed: private or unsafe host not allowed")
	}
	return nil
}

type accountFeedLookupIP func(context.Context, string) ([]net.IPAddr, error)
type accountFeedDial func(context.Context, string, string) (net.Conn, error)

func accountFeedHTTPTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return newAccountFeedHTTPTransport(
		net.DefaultResolver.LookupIPAddr,
		dialer.DialContext,
	)
}

func newAccountFeedHTTPTransport(lookup accountFeedLookupIP, dial accountFeedDial) *http.Transport {
	return &http.Transport{
		// Environment-configured proxies could connect to an unvalidated internal
		// destination, so feeds always use a direct connection.
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("accountfeed: invalid feed network address: %w", err)
			}
			resolved, err := lookup(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("accountfeed: resolve feed host: %w", err)
			}
			if len(resolved) == 0 {
				return nil, fmt.Errorf("accountfeed: feed host resolved to no addresses")
			}
			for _, candidate := range resolved {
				if accountFeedAddressBlocked(candidate.IP) {
					return nil, fmt.Errorf("accountfeed: feed host resolved to blocked address space")
				}
			}
			return dial(ctx, network, net.JoinHostPort(resolved[0].IP.String(), port))
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	}
}

func accountFeedAddressBlocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	if ip.IsLoopback() {
		return false
	}
	return ip.IsUnspecified() ||
		ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() ||
		ip.String() == "169.254.169.254"
}
