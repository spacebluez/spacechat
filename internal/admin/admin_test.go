package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"xchat/internal/protocol"
)

type cleaner struct {
	calls int
	fail  bool
}

func (target *cleaner) ClearHistory() (protocol.Cleared, error) {
	target.calls++
	if target.fail {
		return protocol.Cleared{}, errors.New("injected failure")
	}
	return protocol.Cleared{InstanceID: "new", Deleted: 3}, nil
}
func TestAdministrativeHandler(t *testing.T) {
	target := &cleaner{}
	handler := Handler(target)
	for _, test := range []struct {
		method, path string
		code         int
	}{{"GET", "/clear-history", 405}, {"POST", "/unknown", 404}, {"POST", "/clear-history", 200}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.code {
			t.Fatalf("%s %s: %d", test.method, test.path, response.Code)
		}
	}
	if target.calls != 1 {
		t.Fatal("unexpected clear calls")
	}
	target.fail = true
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/clear-history", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatal("failure not propagated")
	}
}
