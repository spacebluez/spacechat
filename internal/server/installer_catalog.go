package server

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"xchat/internal/update"
)

type installerPackage struct {
	artifact update.Artifact
	data     []byte
}

type InstallerCatalog struct {
	rawManifest []byte
	signature   []byte
	manifest    update.InstallerManifest
	origin      string
	files       map[string]installerPackage
}

func WithInstallerCatalog(catalog *InstallerCatalog) RoomsOption {
	return func(service *Server) {
		service.installers = catalog
	}
}

func LoadInstallerCatalog(directory string, publicKey ed25519.PublicKey) (*InstallerCatalog, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, fmt.Errorf("installer directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("installer directory must be a real directory")
	}
	raw, err := readRegularFile(filepath.Join(directory, "installers.json"), 1<<20)
	if err != nil {
		return nil, fmt.Errorf("installer manifest: %w", err)
	}
	signature, err := readRegularFile(filepath.Join(directory, "installers.sig"), ed25519.SignatureSize)
	if err != nil {
		return nil, fmt.Errorf("installer manifest signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return nil, errors.New("installer manifest signature has invalid size")
	}
	manifest, err := update.VerifyInstallerManifest(raw, signature, publicKey)
	if err != nil {
		return nil, err
	}
	origin, err := installerOrigin(manifest.Server)
	if err != nil {
		return nil, err
	}
	files := make(map[string]installerPackage, len(manifest.Packages))
	for _, item := range manifest.Packages {
		data, readErr := readRegularFile(filepath.Join(directory, item.File), update.MaxArtifactSize)
		if readErr != nil {
			return nil, fmt.Errorf("read installer package %s: %w", item.File, readErr)
		}
		digest := sha256.Sum256(data)
		if int64(len(data)) != item.Size || hex.EncodeToString(digest[:]) != item.SHA256 {
			err = errors.New("size or digest does not match manifest")
			return nil, fmt.Errorf("verify installer package %s: %w", item.File, err)
		}
		files[item.File] = installerPackage{artifact: item, data: data}
	}
	return &InstallerCatalog{
		rawManifest: append([]byte(nil), raw...),
		signature:   append([]byte(nil), signature...),
		manifest:    manifest,
		origin:      origin,
		files:       files,
	}, nil
}

func (catalog *InstallerCatalog) Manifest() update.InstallerManifest {
	copyOf := catalog.manifest
	copyOf.Packages = make(map[string]update.Artifact, len(catalog.manifest.Packages))
	for platform, item := range catalog.manifest.Packages {
		copyOf.Packages[platform] = item
	}
	return copyOf
}

func (catalog *InstallerCatalog) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /install/v1/manifest", func(writer http.ResponseWriter, request *http.Request) {
		serveUpdateBytes(writer, catalog.rawManifest, false)
	})
	mux.HandleFunc("GET /install/v1/manifest.sig", func(writer http.ResponseWriter, request *http.Request) {
		serveUpdateBytes(writer, catalog.signature, false)
	})
	mux.HandleFunc("GET /install/v1/packages/{name}", func(writer http.ResponseWriter, request *http.Request) {
		catalog.servePackage(writer, request)
	})
	mux.HandleFunc("GET /install/linux", func(writer http.ResponseWriter, request *http.Request) {
		item, _ := catalog.manifest.Package("linux", "amd64")
		serveInstallerScript(writer, "text/x-shellscript; charset=utf-8", linuxBootstrap(catalog.origin, catalog.manifest, item))
	})
	mux.HandleFunc("GET /install/windows", func(writer http.ResponseWriter, request *http.Request) {
		item, _ := catalog.manifest.Package("windows", "amd64")
		serveInstallerScript(writer, "text/plain; charset=utf-8", windowsBootstrap(catalog.origin, catalog.manifest, item))
	})
}

func (catalog *InstallerCatalog) servePackage(writer http.ResponseWriter, request *http.Request) {
	name := request.PathValue("name")
	item, exists := catalog.files[name]
	if !exists {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "application/zip")
	writer.Header().Set("Content-Disposition", `attachment; filename="`+item.artifact.File+`"`)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	writer.Header().Set("Content-Length", strconv.FormatInt(item.artifact.Size, 10))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(item.data)
}

func installerOrigin(server string) (string, error) {
	origin, err := url.Parse(server)
	if err != nil || origin.Host == "" {
		return "", errors.New("invalid installer server URL")
	}
	switch origin.Scheme {
	case "ws":
		origin.Scheme = "http"
	case "wss":
		origin.Scheme = "https"
	default:
		return "", errors.New("invalid installer server scheme")
	}
	origin.Path = ""
	origin.RawPath = ""
	origin.RawQuery = ""
	origin.ForceQuery = false
	origin.Fragment = ""
	result := origin.String()
	if result == "" {
		return "", errors.New("invalid request origin")
	}
	return result, nil
}

func serveInstallerScript(writer http.ResponseWriter, contentType string, script []byte) {
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Length", strconv.Itoa(len(script)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(script)
}

func shellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func powershellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func linuxBootstrap(origin string, manifest update.InstallerManifest, item update.Artifact) []byte {
	packageURL := origin + "/install/v1/packages/" + item.File
	protocol := "=http"
	if strings.HasPrefix(origin, "https://") {
		protocol = "=https"
	}
	script := fmt.Sprintf(`#!/bin/sh
set -eu
package_url=%s
expected_sha256=%s
server=%s
version=%s
for command in curl unzip sha256sum mktemp; do
    command -v "$command" >/dev/null || { printf 'Required command not found: %%s\n' "$command" >&2; exit 1; }
done
temporary=$(mktemp -d "${TMPDIR:-/tmp}/spacechat-install.XXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
archive="$temporary/package.zip"
package_directory="$temporary/package"
curl --fail --silent --show-error --proto %s --output "$archive" "$package_url"
actual_sha256=$(sha256sum "$archive")
actual_sha256=${actual_sha256%%%% *}
if [ "$actual_sha256" != "$expected_sha256" ]; then
    printf 'SpaceChat installer package checksum mismatch.\n' >&2
    exit 1
fi
mkdir -m 0700 "$package_directory"
unzip -q "$archive" -d "$package_directory"
sh "$package_directory/install.sh" "$server" "$version" "$package_directory"
`, shellLiteral(packageURL), shellLiteral(item.SHA256), shellLiteral(manifest.Server), shellLiteral(manifest.Version), shellLiteral(protocol))
	return []byte(script)
}

func windowsBootstrap(origin string, manifest update.InstallerManifest, item update.Artifact) []byte {
	packageURL := origin + "/install/v1/packages/" + item.File
	script := fmt.Sprintf(`$ErrorActionPreference = "Stop"
$packageUrl = %s
$expectedSha256 = %s
$server = %s
$version = %s
$temporary = Join-Path ([IO.Path]::GetTempPath()) ("spacechat-install-" + [Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Path $temporary | Out-Null
    $archive = Join-Path $temporary "package.zip"
    $packageDirectory = Join-Path $temporary "package"
    Invoke-WebRequest -UseBasicParsing -MaximumRedirection 0 -Uri $packageUrl -OutFile $archive
    $actualSha256 = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualSha256 -ne $expectedSha256) { throw "SpaceChat installer package checksum mismatch" }
    Expand-Archive -LiteralPath $archive -DestinationPath $packageDirectory
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $packageDirectory "install.ps1") -Server $server -Version $version -Source $packageDirectory
    if ($LASTEXITCODE -ne 0) { throw "SpaceChat installer failed with exit code $LASTEXITCODE" }
}
finally {
    if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Recurse -Force }
}
`, powershellLiteral(packageURL), powershellLiteral(item.SHA256), powershellLiteral(manifest.Server), powershellLiteral(manifest.Version))
	return []byte(script)
}
