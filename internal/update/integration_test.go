package update_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"xchat/internal/protocol"
	"xchat/internal/server"
	"xchat/internal/update"
)

type integrationUI struct {
	actions []update.Action
	errors  []error
}

func (ui *integrationUI) Choose(update.Prompt) update.Action {
	action := ui.actions[0]
	ui.actions = ui.actions[1:]
	return action
}

func (ui *integrationUI) Progress(int64, int64) {}
func (ui *integrationUI) ShowError(err error)   { ui.errors = append(ui.errors, err) }

type integrationRelease struct {
	directory string
	publicKey ed25519.PublicKey
	raw       []byte
	signature []byte
	linux     []byte
	linuxName string
}

func createIntegrationRelease(t *testing.T) integrationRelease {
	t.Helper()
	directory := t.TempDir()
	linux := []byte("#!/bin/sh\n[ \"$1\" = \"--self-check\" ]\n")
	windows := []byte("windows executable fixture")
	digest := func(data []byte) string {
		hash := sha256.Sum256(data)
		return hex.EncodeToString(hash[:])
	}
	linuxName := "spacechat-client-linux-amd64-0.4.0"
	windowsName := "spacechat-client-windows-amd64-0.4.0.exe"
	manifest := update.Manifest{
		Schema:                  1,
		LatestVersion:           "0.4.0",
		MinimumSupportedVersion: "0.3.2",
		ReleaseNotes:            "integration release",
		Artifacts: map[string]update.Artifact{
			"linux-amd64":   {File: linuxName, Size: int64(len(linux)), SHA256: digest(linux)},
			"windows-amd64": {File: windowsName, Size: int64(len(windows)), SHA256: digest(windows)},
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, raw)
	for name, data := range map[string][]byte{
		"manifest.json": raw,
		"manifest.sig":  signature,
		linuxName:       linux,
		windowsName:     windows,
	} {
		if err = os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return integrationRelease{directory: directory, publicKey: publicKey, raw: raw, signature: signature, linux: linux, linuxName: linuxName}
}

func integrationLayout(t *testing.T, current string) update.Layout {
	t.Helper()
	root := t.TempDir()
	layout := update.Layout{
		Root:     root,
		Current:  filepath.Join(root, "current"),
		Versions: filepath.Join(root, "versions"),
		Temp:     filepath.Join(root, "tmp"),
		Lock:     filepath.Join(root, "update.lock"),
	}
	version, err := update.ParseVersion(current)
	if err != nil {
		t.Fatal(err)
	}
	if err = update.WriteCurrent(layout, version); err != nil {
		t.Fatal(err)
	}
	return layout
}

func wsAddress(httpServer *httptest.Server) string {
	return "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
}

func TestManagedClientUpdateEndToEnd(t *testing.T) {
	release := createIntegrationRelease(t)
	catalog, err := server.LoadUpdateCatalog(release.directory, release.publicKey)
	if err != nil {
		t.Fatal(err)
	}
	service := server.NewRooms(nil, server.WithUpdateCatalog(catalog))
	t.Cleanup(service.Close)
	httpServer := httptest.NewServer(service.Handler())
	t.Cleanup(httpServer.Close)
	address := wsAddress(httpServer)
	remote := update.Remote{PublicKey: release.publicKey}

	t.Run("optional skip and update", func(t *testing.T) {
		current, _ := update.ParseVersion("0.3.2")
		layout := integrationLayout(t, current.String())
		installer := update.Installer{Layout: layout, Remote: remote, GOOS: "linux"}

		skipUI := &integrationUI{actions: []update.Action{update.ActionSkip}}
		outcome, runError := (update.Runner{Checker: remote, Installer: installer, UI: skipUI}).Run(context.Background(), update.Request{
			Server: address, Current: current, GOOS: "linux", GOARCH: "amd64",
		})
		if runError != nil || outcome != update.Continue {
			t.Fatalf("skip outcome=%v err=%v", outcome, runError)
		}
		active, readError := update.ReadCurrent(layout)
		if readError != nil || active != current {
			t.Fatalf("skip changed current to %v: %v", active, readError)
		}

		launched := false
		updateUI := &integrationUI{actions: []update.Action{update.ActionUpdate}}
		outcome, runError = (update.Runner{
			Checker: remote, Installer: installer, UI: updateUI,
			Launch: func(path string, args []string) error {
				contents, readError := os.ReadFile(path)
				launched = readError == nil && string(contents) == string(release.linux)
				return readError
			},
		}).Run(context.Background(), update.Request{Server: address, Current: current, GOOS: "linux", GOARCH: "amd64"})
		if runError != nil || outcome != update.Relaunched || !launched {
			t.Fatalf("update outcome=%v err=%v launched=%v", outcome, runError, launched)
		}
		active, readError = update.ReadCurrent(layout)
		if readError != nil || active.String() != "0.4.0" {
			t.Fatalf("updated current=%v err=%v", active, readError)
		}
	})

	t.Run("required corrupt artifact remains blocked", func(t *testing.T) {
		corruptServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/updates/v1/manifest":
				_, _ = writer.Write(release.raw)
			case "/updates/v1/manifest.sig":
				_, _ = writer.Write(release.signature)
			case "/updates/v1/artifacts/" + release.linuxName:
				corrupt := append([]byte(nil), release.linux...)
				corrupt[len(corrupt)-1] ^= 1
				_, _ = writer.Write(corrupt)
			default:
				http.NotFound(writer, request)
			}
		}))
		defer corruptServer.Close()
		current, _ := update.ParseVersion("0.3.1")
		layout := integrationLayout(t, current.String())
		ui := &integrationUI{actions: []update.Action{update.ActionUpdate, update.ActionExit}}
		installer := update.Installer{Layout: layout, Remote: remote, GOOS: "linux"}
		outcome, runError := (update.Runner{Checker: remote, Installer: installer, UI: ui}).Run(context.Background(), update.Request{
			Server: wsAddress(corruptServer), Current: current, GOOS: "linux", GOARCH: "amd64",
		})
		if runError != nil || outcome != update.Exit || len(ui.errors) != 1 {
			t.Fatalf("corrupt outcome=%v err=%v displayed=%v", outcome, runError, ui.errors)
		}
		active, readError := update.ReadCurrent(layout)
		if readError != nil || active != current {
			t.Fatalf("corrupt update changed current to %v: %v", active, readError)
		}
		targetVersion, _ := update.ParseVersion("0.4.0")
		target, _ := layout.ClientPath(targetVersion, "linux")
		if _, statError := os.Lstat(target); !os.IsNotExist(statError) {
			t.Fatalf("corrupt target remains: %v", statError)
		}
	})

	t.Run("websocket rejects old client before history", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		connection, _, dialError := websocket.Dial(ctx, address, nil)
		if dialError != nil {
			t.Fatal(dialError)
		}
		defer connection.CloseNow()
		join := protocol.Join{Nickname: "old-client", AccessKey: "room-key", ClientVersion: "0.3.1", ClientOS: "linux", ClientArch: "amd64"}
		if writeError := wsjson.Write(ctx, connection, protocol.Encode("join", "join", join)); writeError != nil {
			t.Fatal(writeError)
		}
		var frame protocol.Frame
		if readError := wsjson.Read(ctx, connection, &frame); readError != nil {
			t.Fatal(readError)
		}
		if frame.Type != "error" {
			t.Fatalf("first server frame = %s, wanted upgrade error before history", frame.Type)
		}
		var failure protocol.Failure
		if decodeError := json.Unmarshal(frame.Payload, &failure); decodeError != nil {
			t.Fatal(decodeError)
		}
		if failure.Code != "upgrade_required" || failure.MinimumVersion != "0.3.2" {
			t.Fatalf("failure = %+v", failure)
		}
	})
}
