//go:build !darwin

package services

// port443_other.go — non-darwin stub for listenTLS / SetPrivilegedPrelude.
//
// Windows / Linux don't need the macOS porthelper fd-passing trick:
//   - Windows runs elevated via UAC and binds 443 directly.
//   - Linux users bind 443 via CAP_NET_BIND_SERVICE, sudo, or by choosing
//     an unprivileged port; plain tls.Listen works for both cases.
//
// These stubs keep the cross-platform build green without pulling in the
// darwin-only embedded helper.

import (
	"crypto/tls"
	"fmt"
	"net"
)

// SetPrivilegedPrelude is a no-op on non-darwin platforms (no osascript to
// splice a privileged prelude into).
func SetPrivilegedPrelude(script string) {
	_ = script
}

// listenTLS returns a TLS listener on 127.0.0.1:<port> using plain
// tls.Listen. The caller is responsible for having the necessary privilege
// to bind to the requested port.
func listenTLS(port int, tlsConfig *tls.Config) (net.Listener, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := tls.Listen("tcp4", addr, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("tls.Listen %s: %w", addr, err)
	}
	return ln, nil
}
