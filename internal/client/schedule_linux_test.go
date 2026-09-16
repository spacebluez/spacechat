package client

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xchat/internal/admin"
	"xchat/internal/server"
	"xchat/internal/store"
)

func TestSystemdTimerClearsIsolatedRoom(t *testing.T) {
	if os.Getenv("XCHAT_SYSTEMD_TEST") != "1" {
		t.Skip("requires explicit permission to create an isolated transient systemd timer")
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	repository, err := store.Open(filepath.Join(directory, "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	repository.Append("seed", "isolated timer test")
	instance := repository.InstanceID()
	service := server.New(repository, "test-access-key")
	defer service.Close()
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	socket := filepath.Join(directory, "admin.sock")
	management, err := admin.Start(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	defer management.Close()
	network := New("ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { network.Run(ctx, "observer", "test-access-key"); close(done) }()
	defer func() { cancel(); <-done }()
	awaitEvent(t, network.Events(), func(event Event) bool { return event.State == "connected" })
	script, err := filepath.Abs("../../deploy/cleanup.sh")
	if err != nil {
		t.Fatal(err)
	}
	unit := fmt.Sprintf("xchat-cleanup-test-%d", time.Now().UnixNano())
	defer exec.Command("systemctl", "stop", unit+".timer", unit+".service").Run()
	command := exec.Command("systemd-run", "--unit="+unit, "--on-active=2s", "--timer-property=AccuracySec=1s", "--property=Environment=XCHAT_ADMIN_SOCKET="+socket, "/bin/sh", script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("start test timer: %v: %s", err, output)
	}
	awaitEvent(t, network.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "history_cleared" })
	latest, err := repository.LatestID()
	if err != nil || latest != 0 || repository.InstanceID() == instance {
		t.Fatal("timer did not clear room")
	}
	if err = network.Send("after-timer", "still connected"); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, network.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "ack" })
	latest, _ = repository.LatestID()
	if latest != 2 {
		t.Fatal("post-cleanup message was not saved")
	}
	t.Log("real systemd timer invoked cleanup.sh; isolated room cleared and client remained connected")
}
