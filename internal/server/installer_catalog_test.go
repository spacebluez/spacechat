package server

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"xchat/internal/update"
)

func fakeLinuxInstallerPackage(t *testing.T) []byte {
	t.Helper()
	contents := new(bytes.Buffer)
	archive := zip.NewWriter(contents)
	installer, err := archive.Create("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = installer.Write([]byte("#!/bin/sh\nset -eu\nprintf '%s\\n%s\\n' \"$1\" \"$2\" > \"$HOME/install-result\"\n")); err != nil {
		t.Fatal(err)
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	return contents.Bytes()
}

type installerCatalogFixture struct {
	directory string
	publicKey ed25519.PublicKey
	raw       []byte
	signature []byte
	files     map[string][]byte
}

func makeInstallerCatalogFixture(t *testing.T) installerCatalogFixture {
	return makeInstallerCatalogFixtureForServer(t, 1, "ws://192.168.33.216:18081/ws")
}

func makeInstallerCatalogFixtureForServer(t *testing.T, schema int, server string) installerCatalogFixture {
	t.Helper()
	directory := t.TempDir()
	files := map[string][]byte{
		"spacechat-windows-amd64-0.4.0.zip": []byte("windows installer"),
		"spacechat-linux-amd64-0.4.0.zip":   fakeLinuxInstallerPackage(t),
	}
	digest := func(data []byte) string {
		hash := sha256.Sum256(data)
		return hex.EncodeToString(hash[:])
	}
	manifest := update.InstallerManifest{
		Schema: schema, Version: "0.4.0", Server: server,
		Packages: map[string]update.Artifact{
			"windows-amd64": {File: "spacechat-windows-amd64-0.4.0.zip", Size: int64(len(files["spacechat-windows-amd64-0.4.0.zip"])), SHA256: digest(files["spacechat-windows-amd64-0.4.0.zip"])},
			"linux-amd64":   {File: "spacechat-linux-amd64-0.4.0.zip", Size: int64(len(files["spacechat-linux-amd64-0.4.0.zip"])), SHA256: digest(files["spacechat-linux-amd64-0.4.0.zip"])},
		},
	}
	if schema == 2 {
		manifest.Server = ""
		manifest.ServerMode = update.InstallerServerModeRequest
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, raw)
	for name, data := range files {
		if err = os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(directory, "installers.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "installers.sig"), signature, 0600); err != nil {
		t.Fatal(err)
	}
	return installerCatalogFixture{directory: directory, publicKey: publicKey, raw: raw, signature: signature, files: files}
}

func loadDynamicFixture(t *testing.T) *InstallerCatalog {
	t.Helper()
	fixture := makeInstallerCatalogFixtureForServer(t, 2, "")
	catalog, err := LoadInstallerCatalog(fixture.directory, fixture.publicKey)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestLinuxBootstrapDownloadsVerifiesAndRunsInstaller(t *testing.T) {
	var packageData []byte
	packageHost := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/install/v1/packages/spacechat-linux-amd64-0.4.0.zip" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write(packageData)
	}))
	defer packageHost.Close()
	server := "ws" + strings.TrimPrefix(packageHost.URL, "http") + "/ws"
	fixture := makeInstallerCatalogFixtureForServer(t, 1, server)
	packageData = fixture.files["spacechat-linux-amd64-0.4.0.zip"]
	catalog, err := LoadInstallerCatalog(fixture.directory, fixture.publicKey)
	if err != nil {
		t.Fatal(err)
	}
	service := NewRooms(nil, WithInstallerCatalog(catalog))
	defer service.Close()
	recorder := httptest.NewRecorder()
	service.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/install/linux", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d", recorder.Code)
	}
	home := t.TempDir()
	command := exec.Command("sh")
	command.Stdin = recorder.Body
	command.Env = append(os.Environ(), "HOME="+home, "TMPDIR="+t.TempDir())
	if output, runError := command.CombinedOutput(); runError != nil {
		t.Fatalf("bootstrap failed: %v\n%s", runError, output)
	}
	result, err := os.ReadFile(filepath.Join(home, "install-result"))
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != server+"\n0.4.0\n" {
		t.Fatalf("installer invocation = %q", result)
	}
}

func TestDynamicInstallerAddressesUseRequestOrExplicitURL(t *testing.T) {
	tests := []struct {
		name, requestURL, publicURL, expectedOrigin, expectedServer string
	}{
		{"http request", "http://10.0.0.8:18081/install/linux", "", "http://10.0.0.8:18081", "ws://10.0.0.8:18081/ws"},
		{"https request", "https://chat.example/install/linux", "", "https://chat.example", "wss://chat.example/ws"},
		{"proxy override", "http://server:18081/install/linux", "wss://chat.example/ws", "https://chat.example", "wss://chat.example/ws"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := loadDynamicFixture(t)
			if err := catalog.SetPublicURL(test.publicURL); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, test.requestURL, nil)
			request.Header.Set("X-Forwarded-Proto", "https")
			recorder := httptest.NewRecorder()
			NewRooms(nil, WithInstallerCatalog(catalog)).Handler().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.expectedOrigin) || !strings.Contains(recorder.Body.String(), test.expectedServer) {
				t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestDynamicInstallerAddressesRejectMaliciousRequestHost(t *testing.T) {
	for _, host := range []string{"user@host", "chat.example bad", "chat.example/install", `chat.example\install`} {
		t.Run(host, func(t *testing.T) {
			catalog := loadDynamicFixture(t)
			request := httptest.NewRequest(http.MethodGet, "http://chat.example/install/linux", nil)
			request.Host = host
			recorder := httptest.NewRecorder()
			NewRooms(nil, WithInstallerCatalog(catalog)).Handler().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("host %q status=%d body=%q", host, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestFixedInstallerAddressesIgnoreRequestHost(t *testing.T) {
	fixture := makeInstallerCatalogFixture(t)
	catalog, err := LoadInstallerCatalog(fixture.directory, fixture.publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err = catalog.SetPublicURL("wss://override.example/ws"); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://chat.example/install/linux", nil)
	request.Host = "user@host"
	recorder := httptest.NewRecorder()
	NewRooms(nil, WithInstallerCatalog(catalog)).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "http://192.168.33.216:18081") || !strings.Contains(recorder.Body.String(), "ws://192.168.33.216:18081/ws") {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestSetPublicURLRejectsInvalidURL(t *testing.T) {
	for _, raw := range []string{
		"http://chat.example/ws",
		"wss://user@chat.example/ws",
		"wss://chat.example/ws#fragment",
		"wss:///ws",
		"wss://chat.example/not-ws",
		"wss://chat.example/ws?query",
	} {
		t.Run(raw, func(t *testing.T) {
			if err := loadDynamicFixture(t).SetPublicURL(raw); err == nil {
				t.Fatalf("invalid public URL accepted: %q", raw)
			}
		})
	}
}

func TestInstallerCatalogServesVerifiedPackagesAndBootstrapScripts(t *testing.T) {
	fixture := makeInstallerCatalogFixture(t)
	catalog, err := LoadInstallerCatalog(fixture.directory, fixture.publicKey)
	if err != nil {
		t.Fatal(err)
	}
	service := NewRooms(nil, WithInstallerCatalog(catalog))
	defer service.Close()

	for path, expected := range map[string][]byte{
		"/install/v1/manifest":                                 fixture.raw,
		"/install/v1/manifest.sig":                             fixture.signature,
		"/install/v1/packages/spacechat-linux-amd64-0.4.0.zip": fixture.files["spacechat-linux-amd64-0.4.0.zip"],
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "http://download.invalid"+path, nil)
		service.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), expected) {
			t.Fatalf("GET %s = %d %q", path, recorder.Code, recorder.Body.Bytes())
		}
		if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("GET %s omitted nosniff", path)
		}
	}

	linux := httptest.NewRecorder()
	service.Handler().ServeHTTP(linux, httptest.NewRequest(http.MethodGet, "http://download.invalid/install/linux", nil))
	linuxScript := linux.Body.String()
	linuxPackage := catalog.Manifest().Packages["linux-amd64"]
	for _, expected := range []string{
		"http://192.168.33.216:18081/install/v1/packages/" + linuxPackage.File,
		linuxPackage.SHA256,
		"ws://192.168.33.216:18081/ws",
		"sha256sum",
		"install.sh",
	} {
		if linux.Code != http.StatusOK || !strings.Contains(linuxScript, expected) {
			t.Fatalf("Linux bootstrap missing %q: status=%d body=%q", expected, linux.Code, linuxScript)
		}
	}
	if strings.Contains(linuxScript, "download.invalid") || strings.Contains(linuxScript, "--location") {
		t.Fatalf("Linux bootstrap trusted request host or enabled redirects: %q", linuxScript)
	}
	if linux.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(linux.Header().Get("Content-Type"), "text/x-shellscript") {
		t.Fatalf("Linux bootstrap headers = %v", linux.Header())
	}

	windows := httptest.NewRecorder()
	service.Handler().ServeHTTP(windows, httptest.NewRequest(http.MethodGet, "https://download.invalid/install/windows", nil))
	windowsScript := windows.Body.String()
	windowsPackage := catalog.Manifest().Packages["windows-amd64"]
	for _, expected := range []string{
		"http://192.168.33.216:18081/install/v1/packages/" + windowsPackage.File,
		windowsPackage.SHA256,
		"ws://192.168.33.216:18081/ws",
		"Get-FileHash",
		"-MaximumRedirection 0",
		"install.ps1",
	} {
		if windows.Code != http.StatusOK || !strings.Contains(windowsScript, expected) {
			t.Fatalf("Windows bootstrap missing %q: status=%d body=%q", expected, windows.Code, windowsScript)
		}
	}
	if strings.Contains(windowsScript, "download.invalid") {
		t.Fatalf("Windows bootstrap trusted request host: %q", windowsScript)
	}
}

func TestInstallerCatalogServesValidatedSnapshotAfterPackageChanges(t *testing.T) {
	fixture := makeInstallerCatalogFixture(t)
	catalog, err := LoadInstallerCatalog(fixture.directory, fixture.publicKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fixture.directory, "spacechat-linux-amd64-0.4.0.zip")
	changed := bytes.Repeat([]byte{'x'}, len(fixture.files["spacechat-linux-amd64-0.4.0.zip"]))
	if err = os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	service := NewRooms(nil, WithInstallerCatalog(catalog))
	defer service.Close()
	recorder := httptest.NewRecorder()
	service.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/install/v1/packages/spacechat-linux-amd64-0.4.0.zip", nil))
	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), fixture.files["spacechat-linux-amd64-0.4.0.zip"]) {
		t.Fatalf("package response changed after catalog load: status=%d body=%q", recorder.Code, recorder.Body.Bytes())
	}
}

func TestInstallerCatalogDoesNotServeUnlistedPaths(t *testing.T) {
	fixture := makeInstallerCatalogFixture(t)
	catalog, err := LoadInstallerCatalog(fixture.directory, fixture.publicKey)
	if err != nil {
		t.Fatal(err)
	}
	service := NewRooms(nil, WithInstallerCatalog(catalog))
	defer service.Close()
	for _, path := range []string{
		"/install/v1/packages/not-listed.zip",
		"/install/v1/packages/spacechat-linux-amd64-0.4.0.zip/extra",
		"/install/v1/packages/../installers.json",
	} {
		recorder := httptest.NewRecorder()
		service.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code == http.StatusOK {
			t.Fatalf("GET %s returned installer content", path)
		}
	}
}

func TestInstallerCatalogRejectsInvalidPackages(t *testing.T) {
	tests := map[string]func(*testing.T, installerCatalogFixture){
		"missing": func(t *testing.T, fixture installerCatalogFixture) {
			if err := os.Remove(filepath.Join(fixture.directory, "spacechat-linux-amd64-0.4.0.zip")); err != nil {
				t.Fatal(err)
			}
		},
		"wrong digest": func(t *testing.T, fixture installerCatalogFixture) {
			if err := os.WriteFile(filepath.Join(fixture.directory, "spacechat-linux-amd64-0.4.0.zip"), []byte("corrupt package!"), 0600); err != nil {
				t.Fatal(err)
			}
		},
		"symlink": func(t *testing.T, fixture installerCatalogFixture) {
			path := filepath.Join(fixture.directory, "spacechat-linux-amd64-0.4.0.zip")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(fixture.directory, "spacechat-windows-amd64-0.4.0.zip"), path); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := makeInstallerCatalogFixture(t)
			mutate(t, fixture)
			if _, err := LoadInstallerCatalog(fixture.directory, fixture.publicKey); err == nil {
				t.Fatal("invalid installer catalog accepted")
			}
		})
	}
}
