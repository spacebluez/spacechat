package server

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"xchat/internal/update"
)

type UpdateCatalog struct {
	directory   string
	rawManifest []byte
	signature   []byte
	manifest    update.Manifest
	minimum     update.Version
	files       map[string]update.Artifact
}

type RoomsOption func(*Server)

func WithUpdateCatalog(catalog *UpdateCatalog) RoomsOption {
	return func(service *Server) {
		service.updates = catalog
	}
}

func LoadUpdateCatalog(directory string, publicKey ed25519.PublicKey) (*UpdateCatalog, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, fmt.Errorf("update directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("update directory must be a real directory")
	}
	raw, err := readRegularFile(filepath.Join(directory, "manifest.json"), 1<<20)
	if err != nil {
		return nil, fmt.Errorf("update manifest: %w", err)
	}
	signature, err := readRegularFile(filepath.Join(directory, "manifest.sig"), ed25519.SignatureSize)
	if err != nil {
		return nil, fmt.Errorf("update manifest signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return nil, errors.New("update manifest signature has invalid size")
	}
	manifest, err := update.VerifyManifest(raw, signature, publicKey)
	if err != nil {
		return nil, err
	}
	minimum, err := manifest.Minimum()
	if err != nil {
		return nil, err
	}
	files := make(map[string]update.Artifact, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		path := filepath.Join(directory, artifact.File)
		if err = verifyArtifact(path, artifact); err != nil {
			return nil, fmt.Errorf("verify update artifact %s: %w", artifact.File, err)
		}
		files[artifact.File] = artifact
	}
	return &UpdateCatalog{
		directory:   directory,
		rawManifest: append([]byte(nil), raw...),
		signature:   append([]byte(nil), signature...),
		manifest:    manifest,
		minimum:     minimum,
		files:       files,
	}, nil
}

func readRegularFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("file must be regular and not a symlink")
	}
	if info.Size() > maximum {
		return nil, errors.New("file is too large")
	}
	return os.ReadFile(path)
}

func verifyArtifact(path string, artifact update.Artifact) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("artifact must be regular and not a symlink")
	}
	if info.Size() != artifact.Size {
		return fmt.Errorf("size is %d, expected %d", info.Size(), artifact.Size)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		return errors.New("digest does not match manifest")
	}
	return nil
}

func (catalog *UpdateCatalog) Manifest() update.Manifest {
	copyOf := catalog.manifest
	copyOf.Artifacts = make(map[string]update.Artifact, len(catalog.manifest.Artifacts))
	for name, artifact := range catalog.manifest.Artifacts {
		copyOf.Artifacts[name] = artifact
	}
	return copyOf
}

func (catalog *UpdateCatalog) MinimumVersion() update.Version {
	return catalog.minimum
}

func (catalog *UpdateCatalog) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /updates/v1/manifest", func(writer http.ResponseWriter, request *http.Request) {
		serveUpdateBytes(writer, catalog.rawManifest, false)
	})
	mux.HandleFunc("GET /updates/v1/manifest.sig", func(writer http.ResponseWriter, request *http.Request) {
		serveUpdateBytes(writer, catalog.signature, false)
	})
	mux.HandleFunc("GET /updates/v1/artifacts/{name}", func(writer http.ResponseWriter, request *http.Request) {
		name := request.PathValue("name")
		artifact, exists := catalog.files[name]
		if !exists {
			http.NotFound(writer, request)
			return
		}
		file, err := os.Open(filepath.Join(catalog.directory, artifact.File))
		if err != nil {
			http.Error(writer, "update artifact unavailable", http.StatusServiceUnavailable)
			return
		}
		defer file.Close()
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		writer.Header().Set("Content-Length", strconv.FormatInt(artifact.Size, 10))
		writer.WriteHeader(http.StatusOK)
		_, _ = io.Copy(writer, file)
	})
}

func serveUpdateBytes(writer http.ResponseWriter, data []byte, immutable bool) {
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if immutable {
		writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		writer.Header().Set("Cache-Control", "no-store")
	}
	writer.Header().Set("Content-Length", strconv.Itoa(len(data)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}
