package metrics

import (
	"fmt"
	"net"
	"strings"
)

// RequireLocalhost ensures the provided listen address is bound to localhost only.
//
// We intentionally reject addresses like ":9090" or "0.0.0.0:9090" so metrics
// are not accidentally exposed to the network.
func RequireLocalhost(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen addr %q: %w", addr, err)
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return fmt.Errorf("invalid listen addr %q: host is empty (refusing to bind metrics on all interfaces)", addr)
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	return fmt.Errorf("invalid listen addr %q: host must be localhost/127.0.0.1/::1", addr)
}
