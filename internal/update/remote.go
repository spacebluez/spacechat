package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	metadataRequestTimeout  = 10 * time.Second
	artifactDownloadTimeout = 30 * time.Minute
)

type Remote struct {
	Client    *http.Client
	PublicKey ed25519.PublicKey
}

type Check struct {
	Manifest Manifest
	Artifact Artifact
	Current  Version
	Latest   Version
	Minimum  Version
	Decision Decision
	Server   string
}

func UpdateURL(serverAddress, endpoint string) (*url.URL, error) {
	server, err := url.Parse(serverAddress)
	if err != nil || server.Host == "" || (server.Scheme != "ws" && server.Scheme != "wss") || server.User != nil || server.Fragment != "" {
		return nil, errors.New("server address must use ws or wss with a host")
	}
	target, err := url.Parse(endpoint)
	if err != nil || target.IsAbs() || target.Host != "" || target.RawQuery != "" || target.Fragment != "" || !strings.HasPrefix(target.Path, "/updates/v1/") {
		return nil, errors.New("update endpoint must be a local /updates/v1 path")
	}
	scheme := "http"
	if server.Scheme == "wss" {
		scheme = "https"
	}
	return &url.URL{Scheme: scheme, Host: server.Host, Path: target.Path}, nil
}

func (remote Remote) httpClient(defaultTimeout time.Duration) *http.Client {
	client := http.Client{Timeout: defaultTimeout}
	if remote.Client != nil {
		client = *remote.Client
		if client.Timeout == 0 {
			client.Timeout = defaultTimeout
		}
	}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client
}

func (remote Remote) get(ctx context.Context, serverAddress, endpoint string, maximum int64) ([]byte, error) {
	target, err := UpdateURL(serverAddress, endpoint)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := remote.httpClient(metadataRequestTimeout).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update server returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, errors.New("update response exceeds size limit")
	}
	return data, nil
}

func (remote Remote) Check(ctx context.Context, serverAddress string, current Version, goos, goarch string) (Check, error) {
	raw, err := remote.get(ctx, serverAddress, "/updates/v1/manifest", 1<<20)
	if err != nil {
		return Check{}, err
	}
	signature, err := remote.get(ctx, serverAddress, "/updates/v1/manifest.sig", ed25519.SignatureSize)
	if err != nil {
		return Check{}, err
	}
	manifest, err := VerifyManifest(raw, signature, remote.PublicKey)
	if err != nil {
		return Check{}, err
	}
	artifact, err := manifest.Artifact(goos, goarch)
	if err != nil {
		return Check{}, err
	}
	latest, err := manifest.Latest()
	if err != nil {
		return Check{}, err
	}
	minimum, err := manifest.Minimum()
	if err != nil {
		return Check{}, err
	}
	return Check{
		Manifest: manifest,
		Artifact: artifact,
		Current:  current,
		Latest:   latest,
		Minimum:  minimum,
		Decision: DecisionFor(current, latest, minimum),
		Server:   serverAddress,
	}, nil
}

type progressWriter struct {
	written  int64
	total    int64
	progress func(int64, int64)
	writer   io.Writer
}

func (writer *progressWriter) Write(data []byte) (int, error) {
	count, err := writer.writer.Write(data)
	writer.written += int64(count)
	if writer.progress != nil {
		writer.progress(writer.written, writer.total)
	}
	return count, err
}

func (remote Remote) Download(ctx context.Context, serverAddress string, artifact Artifact, destination string, progress func(received, total int64)) (result error) {
	if artifact.Size <= 0 || artifact.Size > MaxArtifactSize || !digestPattern.MatchString(artifact.SHA256) || filepath.Base(artifact.File) != artifact.File || strings.ContainsAny(artifact.File, `/\`) {
		return errors.New("invalid update artifact metadata")
	}
	target, err := UpdateURL(serverAddress, "/updates/v1/artifacts/"+artifact.File)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return err
	}
	response, err := remote.httpClient(artifactDownloadTimeout).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("update server returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != artifact.Size {
		return fmt.Errorf("artifact response size is %d, expected %d", response.ContentLength, artifact.Size)
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		if result != nil {
			os.Remove(destination)
		}
	}()
	hash := sha256.New()
	writer := &progressWriter{total: artifact.Size, progress: progress, writer: io.MultiWriter(file, hash)}
	written, err := io.Copy(writer, io.LimitReader(response.Body, artifact.Size+1))
	if err != nil {
		return err
	}
	if written != artifact.Size {
		return fmt.Errorf("downloaded %d bytes, expected %d", written, artifact.Size)
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		return errors.New("downloaded artifact digest does not match manifest")
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return nil
}
