package client

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type Options struct {
	RootCAs       *x509.CertPool
	AllowInsecure bool
}

func LoadRootCAs(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return RootCAsFromPEM(data)
}

func RootCAsFromPEM(data []byte) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(data) {
		return nil, errors.New("CA 文件未包含有效的 PEM 证书")
	}
	return pool, nil
}

func ValidateTransport(address string, allowInsecure bool) error {
	if err := ValidateAddress(address); err != nil {
		return err
	}
	parsed, _ := url.Parse(address)
	if parsed.Scheme == "ws" && !allowInsecure {
		ip := net.ParseIP(parsed.Hostname())
		if !strings.EqualFold(parsed.Hostname(), "localhost") && (ip == nil || !ip.IsLoopback()) {
			return errors.New("远程连接必须使用 wss://；明文调试需显式指定 --allow-insecure")
		}
	}
	return nil
}

func (options Options) httpClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: options.RootCAs}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("服务器重定向被拒绝，请直接使用服务器地址")
		},
	}
}

func (options Options) HTTPClient() *http.Client {
	return options.httpClient()
}

// NewPeer keeps this process's identity and certificate policy during room switches.
func (network *Client) NewPeer() *Client {
	peer := NewWithInfo(network.address, network.info, network.options)
	peer.token = network.token
	network.mu.Lock()
	defer network.mu.Unlock()
	if network.catalog != nil {
		catalog := network.catalog.Clone()
		peer.catalog, peer.catalogETag = &catalog, network.catalogETag
	}
	return peer
}
