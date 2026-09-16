package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNetworkRestrictionIgnoresForwardedHeader(t *testing.T) {
	handler, err := RestrictNetworks(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(http.StatusOK) }), "127.0.0.0/8,192.168.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		address string
		status  int
	}{{"192.168.1.50:12345", 200}, {"127.0.0.1:12345", 200}, {"203.0.113.2:12345", 403}} {
		request := httptest.NewRequest("GET", "/healthz", nil)
		request.RemoteAddr = test.address
		request.Header.Set("X-Forwarded-For", "192.168.1.50")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != test.status {
			t.Fatalf("%s: %d", test.address, recorder.Code)
		}
	}
	if _, err = RestrictNetworks(http.NotFoundHandler(), "bad-network"); err == nil {
		t.Fatal("invalid network accepted")
	}
}
