package admin

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateSocketAndCollision(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "admin.sock")
	target := &cleaner{}
	running, err := Start(path, target)
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("socket permissions")
	}
	if second, err := Start(path, target); err == nil {
		second.Close()
		t.Fatal("replaced live socket")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	response, err := client.Post("http://localhost/clear-history", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("clear through socket failed")
	}
	regular := filepath.Join(t.TempDir(), "keep.txt")
	os.WriteFile(regular, []byte("keep"), 0600)
	if other, err := Start(regular, target); err == nil {
		other.Close()
		t.Fatal("replaced regular file")
	}
	contents, _ := os.ReadFile(regular)
	if string(contents) != "keep" {
		t.Fatal("regular file changed")
	}
}
