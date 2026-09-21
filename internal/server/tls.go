package server

import (
	"crypto/tls"
	"errors"
	"net"
)

func TransportConfig(address, certFile, keyFile string, allowInsecure bool) (*tls.Config, error) {
	if (certFile == "") != (keyFile == "") {
		return nil, errors.New("tls-cert and tls-key must be configured together")
	}
	if certFile != "" {
		certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}, nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if !allowInsecure && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("remote listeners require tls-cert and tls-key; plaintext debugging requires allow-insecure")
	}
	return nil, nil
}
