package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"xchat/internal/update"
)

func readPrivateKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1024 {
		return nil, errors.New("private key must be a small regular file, not a symlink")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("private key file permissions must not grant group or other access")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	encoded := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if strings.ContainsAny(encoded, "\r\n\t ") {
		return nil, errors.New("private key file must contain one base64 value")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("private key is not valid base64")
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		derived := ed25519.NewKeyFromSeed(decoded[:ed25519.SeedSize])
		if !bytes.Equal(derived, decoded) {
			return nil, errors.New("private key public half does not match its seed")
		}
		return ed25519.PrivateKey(append([]byte(nil), decoded...)), nil
	default:
		return nil, errors.New("private key must decode to a 32-byte seed or 64-byte Ed25519 key")
	}
}

func writeExclusive(path string, data []byte, mode os.FileMode) (resultError error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() {
		if closeError := file.Close(); resultError == nil {
			resultError = closeError
		}
	}()
	if _, err = file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}

func copyArtifact(source, destination string) (update.Artifact, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return update.Artifact{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > update.MaxArtifactSize {
		return update.Artifact{}, errors.New("artifact must be a non-empty regular file within the size limit")
	}
	input, err := os.Open(source)
	if err != nil {
		return update.Artifact{}, err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return update.Artifact{}, err
	}
	ok := false
	defer func() {
		output.Close()
		if !ok {
			os.Remove(destination)
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, update.MaxArtifactSize+1))
	if err != nil {
		return update.Artifact{}, err
	}
	if written != info.Size() {
		return update.Artifact{}, errors.New("artifact changed while it was copied")
	}
	if err = output.Sync(); err != nil {
		return update.Artifact{}, err
	}
	if err = output.Close(); err != nil {
		return update.Artifact{}, err
	}
	ok = true
	return update.Artifact{File: filepath.Base(destination), Size: written, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func createManifest(versionText, minimumText, keyPath, windowsPath, linuxPath, outputPath string) (resultError error) {
	latest, err := update.ParseVersion(versionText)
	if err != nil {
		return fmt.Errorf("invalid release version: %w", err)
	}
	minimum, err := update.ParseVersion(minimumText)
	if err != nil {
		return fmt.Errorf("invalid minimum version: %w", err)
	}
	if minimum.Compare(latest) > 0 {
		return errors.New("minimum version exceeds release version")
	}
	privateKey, err := readPrivateKey(keyPath)
	if err != nil {
		return fmt.Errorf("read private key: %w", err)
	}
	if outputPath == "" || filepath.Base(filepath.Clean(outputPath)) == "." {
		return errors.New("output directory is required")
	}
	if _, err = os.Lstat(outputPath); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return errors.New("output directory already exists")
		}
		return err
	}
	parent := filepath.Dir(filepath.Clean(outputPath))
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("output parent must be an existing real directory")
	}
	temporary, err := os.MkdirTemp(parent, "."+filepath.Base(outputPath)+"-")
	if err != nil {
		return err
	}
	defer func() {
		if temporary != "" {
			os.RemoveAll(temporary)
		}
	}()
	windowsName := "spacechat-client-windows-amd64-" + latest.String() + ".exe"
	linuxName := "spacechat-client-linux-amd64-" + latest.String()
	windowsArtifact, err := copyArtifact(windowsPath, filepath.Join(temporary, windowsName))
	if err != nil {
		return fmt.Errorf("copy Windows artifact: %w", err)
	}
	linuxArtifact, err := copyArtifact(linuxPath, filepath.Join(temporary, linuxName))
	if err != nil {
		return fmt.Errorf("copy Linux artifact: %w", err)
	}
	manifest := update.Manifest{
		Schema:                  1,
		LatestVersion:           latest.String(),
		MinimumSupportedVersion: minimum.String(),
		ReleaseNotes:            "SpaceChat " + latest.String(),
		Artifacts: map[string]update.Artifact{
			"windows-amd64": windowsArtifact,
			"linux-amd64":   linuxArtifact,
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	signature := ed25519.Sign(privateKey, raw)
	if _, err = update.VerifyManifest(raw, signature, privateKey.Public().(ed25519.PublicKey)); err != nil {
		return fmt.Errorf("verify generated manifest: %w", err)
	}
	if err = writeExclusive(filepath.Join(temporary, "manifest.json"), raw, 0644); err != nil {
		return err
	}
	if err = writeExclusive(filepath.Join(temporary, "manifest.sig"), signature, 0644); err != nil {
		return err
	}
	if err = os.Rename(temporary, outputPath); err != nil {
		return err
	}
	temporary = ""
	return nil
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "spacechat-release: expected public-key or manifest")
		return 2
	}
	switch args[0] {
	case "public-key":
		set := flag.NewFlagSet("public-key", flag.ContinueOnError)
		set.SetOutput(stderr)
		keyPath := set.String("private-key", "", "base64 private key file")
		if err := set.Parse(args[1:]); err != nil || set.NArg() != 0 || *keyPath == "" {
			if err == nil {
				fmt.Fprintln(stderr, "spacechat-release: -private-key is required")
			}
			return 2
		}
		privateKey, err := readPrivateKey(*keyPath)
		if err != nil {
			fmt.Fprintln(stderr, "spacechat-release:", err)
			return 1
		}
		fmt.Fprintln(stdout, base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)))
		return 0
	case "manifest":
		set := flag.NewFlagSet("manifest", flag.ContinueOnError)
		set.SetOutput(stderr)
		version := set.String("version", "", "release version")
		minimum := set.String("minimum", "0.0.0", "minimum supported version")
		keyPath := set.String("private-key", "", "base64 private key file")
		windowsPath := set.String("windows", "", "Windows amd64 client")
		linuxPath := set.String("linux", "", "Linux amd64 client")
		outputPath := set.String("out", "", "output directory")
		if err := set.Parse(args[1:]); err != nil {
			return 2
		}
		if set.NArg() != 0 || *version == "" || *keyPath == "" || *windowsPath == "" || *linuxPath == "" || *outputPath == "" {
			fmt.Fprintln(stderr, "spacechat-release: manifest requires -version, -private-key, -windows, -linux, and -out")
			return 2
		}
		if err := createManifest(*version, *minimum, *keyPath, *windowsPath, *linuxPath, *outputPath); err != nil {
			fmt.Fprintln(stderr, "spacechat-release:", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintln(stderr, "spacechat-release: unknown command")
		return 2
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
