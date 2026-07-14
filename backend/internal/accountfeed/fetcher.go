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
		return os.ReadFile(full)
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
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("accountfeed: feed fetch failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("accountfeed: feed HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes))
	default:
		return nil, fmt.Errorf("accountfeed: unsupported sourceType %q", feed.SourceType)
	}
}

// validateFeedURL rejects link-local, metadata, and unspecified hosts. Localhost
// is allowed only for local dev (§10.11).
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
	if ip := net.ParseIP(host); ip != nil {
		switch {
		case ip.IsLoopback():
			return nil
		case ip.IsUnspecified():
			return fmt.Errorf("accountfeed: unspecified host not allowed")
		case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
			return fmt.Errorf("accountfeed: link-local host not allowed")
		case ip.String() == "169.254.169.254":
			return fmt.Errorf("accountfeed: metadata host not allowed")
		}
	}
	return nil
}
