package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"xchat/internal/kaomoji"
)

func (network *Client) FetchKaomoji(parent context.Context) (kaomoji.Catalog, error) {
	if err := ValidateTransport(network.address, network.options.AllowInsecure); err != nil {
		return kaomoji.Catalog{}, err
	}
	address, _ := url.Parse(network.address)
	if address.Scheme == "wss" {
		address.Scheme = "https"
	} else {
		address.Scheme = "http"
	}
	address = address.ResolveReference(&url.URL{Path: "api/kaomoji"})
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return kaomoji.Catalog{}, err
	}
	network.catalogMu.Lock()
	defer network.catalogMu.Unlock()
	network.mu.Lock()
	cached, etag := network.catalog, network.catalogETag
	network.mu.Unlock()
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	request.Header.Set("Accept", "application/json")
	httpClient := network.options.httpClient()
	defer httpClient.CloseIdleConnections()
	response, err := httpClient.Do(request)
	if err != nil {
		return kaomoji.Catalog{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified && cached != nil {
		return cached.Clone(), nil
	}
	if response.StatusCode != http.StatusOK {
		return kaomoji.Catalog{}, fmt.Errorf("kaomoji catalog: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, kaomoji.MaxCatalogBytes+1))
	if err != nil {
		return kaomoji.Catalog{}, err
	}
	catalog, err := kaomoji.Parse(data)
	if err != nil {
		return kaomoji.Catalog{}, err
	}
	network.mu.Lock()
	network.catalog, network.catalogETag = &catalog, response.Header.Get("ETag")
	network.mu.Unlock()
	return catalog.Clone(), nil
}
