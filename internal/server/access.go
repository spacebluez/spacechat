package server

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

func RestrictNetworks(next http.Handler, networks string) (http.Handler, error) {
	var allowed []netip.Prefix
	for _, entry := range strings.Split(networks, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(entry))
		if err != nil {
			return nil, fmt.Errorf("invalid allowed network: %w", err)
		}
		allowed = append(allowed, prefix)
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		host, _, err := net.SplitHostPort(request.RemoteAddr)
		if err == nil {
			address, err := netip.ParseAddr(host)
			if err == nil {
				for _, prefix := range allowed {
					if prefix.Contains(address.Unmap()) {
						next.ServeHTTP(writer, request)
						return
					}
				}
			}
		}
		http.Error(writer, "network not allowed", http.StatusForbidden)
	}), nil
}
