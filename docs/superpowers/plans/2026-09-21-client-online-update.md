# SpaceChat Client Online Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install a stable `spacechat` command on Windows amd64 and Linux amd64 that securely updates the versioned chat client from the selected SpaceChat server and enforces server-declared minimum client compatibility.

**Architecture:** A minimal stable launcher reads an atomic `current` pointer and starts a versioned client. The versioned client verifies a signed server manifest, prompts before downloading, installs into a new version directory, self-checks, atomically switches versions, and relaunches; the WebSocket join independently rejects clients below the signed minimum version.

**Tech Stack:** Go 1.26, `net/http`, `crypto/ed25519`, SHA-256, Bubble Tea, PowerShell, POSIX shell, systemd.

**Spec:** `docs/superpowers/specs/2026-09-21-client-online-update-design.md`

## Global Constraints

- Support only Windows amd64 and Linux amd64 in the first release.
- Update metadata and artifacts come only from the SpaceChat server selected for this invocation.
- The stable launcher never performs network operations and never executes a path outside its managed `versions` directory.
- Versions are strict `MAJOR.MINOR.PATCH` decimal values with no leading zero, prerelease, or build suffix.
- The client verifies an Ed25519 signature over the exact manifest bytes before parsing or using metadata.
- Release signing private keys never enter the repository, deployment package, server filesystem, or logs.
- Optional updates offer `Update / Skip`; required updates offer `Update / Exit`; no download starts before confirmation.
- Failed updates preserve the active version; a failed required update never enters chat.
- `Skip` applies only to the current process.
- The server repeats minimum-version enforcement during WebSocket join before reading room history or presence.
- No differential download, resume, silent background update, ARM build, automatic downgrade, or external package registry is included.

---

### Task 1: Strict Versions and Signed Manifest Model

**Files:**
- Create: `internal/update/version.go`
- Create: `internal/update/manifest.go`
- Create: `internal/update/version_test.go`
- Create: `internal/update/manifest_test.go`

**Interfaces:**
- Produces: `ParseVersion(string) (Version, error)`, `Version.Compare(Version) int`, `Version.String() string`.
- Produces: `DecisionFor(current, latest, minimum Version) Decision` with `DecisionCurrent`, `DecisionOptional`, `DecisionRequired`, and `DecisionAhead`.
- Produces: `VerifyManifest(raw, signature []byte, publicKey ed25519.PublicKey) (Manifest, error)`.
- Produces: `Manifest.Artifact(goos, goarch string) (Artifact, error)` and `MaxArtifactSize = 128 << 20`.

- [ ] **Step 1: Write failing strict-version tests**

```go
func TestParseVersionAndDecision(t *testing.T) {
    valid := map[string]string{"0.0.0": "0.0.0", "0.4.0": "0.4.0", "12.34.56": "12.34.56"}
    for input, expected := range valid {
        version, err := ParseVersion(input)
        if err != nil || version.String() != expected { t.Fatalf("%q: %v %v", input, version, err) }
    }
    for _, input := range []string{"", "v1.2.3", "01.2.3", "1.2", "1.2.3-beta", "1.2.3.4", "-1.2.3"} {
        if _, err := ParseVersion(input); err == nil { t.Fatalf("accepted %q", input) }
    }
    minimum, _ := ParseVersion("0.3.2")
    latest, _ := ParseVersion("0.4.0")
    cases := map[string]Decision{"0.3.1": DecisionRequired, "0.3.2": DecisionOptional, "0.4.0": DecisionCurrent, "0.5.0": DecisionAhead}
    for text, expected := range cases {
        current, _ := ParseVersion(text)
        if actual := DecisionFor(current, latest, minimum); actual != expected { t.Fatalf("%s: %v", text, actual) }
    }
}
```

- [ ] **Step 2: Run the version test and verify RED**

Run: `go test ./internal/update -run TestParseVersionAndDecision -count=1`

Expected: FAIL because `ParseVersion` and `DecisionFor` do not exist.

- [ ] **Step 3: Implement strict versions and decisions**

```go
type Version struct { Major, Minor, Patch uint64 }
type Decision uint8
const (
    DecisionCurrent Decision = iota
    DecisionOptional
    DecisionRequired
    DecisionAhead
)

func DecisionFor(current, latest, minimum Version) Decision {
    if current.Compare(minimum) < 0 { return DecisionRequired }
    if current.Compare(latest) < 0 { return DecisionOptional }
    if current.Compare(latest) > 0 { return DecisionAhead }
    return DecisionCurrent
}
```

Parse with a three-capture regular expression, reject leading zeroes except the single digit `0`, and use `strconv.ParseUint` with overflow errors preserved.

- [ ] **Step 4: Write failing manifest verification tests**

```go
func TestVerifyManifestAndSelectArtifact(t *testing.T) {
    publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
    raw := []byte(`{"schema":1,"latest_version":"0.4.0","minimum_supported_version":"0.3.2","release_notes":"online update","artifacts":{"windows-amd64":{"file":"spacechat-client-windows-amd64-0.4.0.exe","size":4,"sha256":"88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589"},"linux-amd64":{"file":"spacechat-client-linux-amd64-0.4.0","size":4,"sha256":"88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589"}}}`)
    manifest, err := VerifyManifest(raw, ed25519.Sign(privateKey, raw), publicKey)
    if err != nil { t.Fatal(err) }
    artifact, err := manifest.Artifact("windows", "amd64")
    if err != nil || artifact.Size != 4 { t.Fatal(artifact, err) }
    raw[10] ^= 1
    if _, err = VerifyManifest(raw, ed25519.Sign(privateKey, raw[:10]), publicKey); err == nil { t.Fatal("accepted invalid signature") }
}
```

Add table cases for unknown JSON fields, trailing JSON, schema other than `1`, minimum newer than latest, missing platform, unsafe filenames, uppercase or malformed digest, zero/oversized files, unsupported OS/architecture, and control characters in release notes.

- [ ] **Step 5: Run manifest tests and verify RED**

Run: `go test ./internal/update -run 'TestVerifyManifest|TestManifestValidation' -count=1`

Expected: FAIL because the manifest API does not exist.

- [ ] **Step 6: Implement signed manifest parsing**

```go
type Artifact struct {
    File string `json:"file"`
    Size int64 `json:"size"`
    SHA256 string `json:"sha256"`
}
type Manifest struct {
    Schema int `json:"schema"`
    LatestVersion string `json:"latest_version"`
    MinimumSupportedVersion string `json:"minimum_supported_version"`
    ReleaseNotes string `json:"release_notes"`
    Artifacts map[string]Artifact `json:"artifacts"`
}
const MaxArtifactSize int64 = 128 << 20
```

Verify key and signature lengths, call `ed25519.Verify` before JSON decoding, use `json.Decoder.DisallowUnknownFields`, require EOF, validate both platform entries, and require `filepath.Base(file) == file` plus no slash or backslash.

- [ ] **Step 7: Run the package tests and commit**

Run: `go test ./internal/update -count=1`

Expected: PASS.

```bash
git add internal/update
git commit -m "feat:新增客户端版本与更新清单模型"
```

### Task 2: Server Update Catalog and HTTP Distribution

**Files:**
- Create: `internal/server/update_catalog.go`
- Create: `internal/server/update_catalog_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/rooms.go`

**Interfaces:**
- Consumes: `update.VerifyManifest`, `update.Manifest`, and `update.Version` from Task 1.
- Produces: `LoadUpdateCatalog(dir string, publicKey ed25519.PublicKey) (*UpdateCatalog, error)`.
- Produces: `WithUpdateCatalog(*UpdateCatalog) RoomsOption`; existing `NewRooms(repository)` calls remain valid through a variadic option.
- Produces: immutable `UpdateCatalog.Manifest() update.Manifest` and `UpdateCatalog.MinimumVersion() update.Version`.

- [ ] **Step 1: Write failing catalog load and HTTP tests**

Create a temporary signed manifest, signature, and two four-byte artifacts. Assert:

```go
catalog, err := LoadUpdateCatalog(directory, publicKey)
if err != nil { t.Fatal(err) }
service := NewRooms(repository, WithUpdateCatalog(catalog))
server := httptest.NewServer(service.Handler())
defer server.Close()
for path, expected := range map[string][]byte{
    "/updates/v1/manifest": raw,
    "/updates/v1/manifest.sig": signature,
    "/updates/v1/artifacts/spacechat-client-linux-amd64-0.4.0": []byte("test"),
} {
    response, _ := http.Get(server.URL + path)
    body, _ := io.ReadAll(response.Body)
    if response.StatusCode != http.StatusOK || !bytes.Equal(body, expected) { t.Fatalf("%s", path) }
}
```

Also assert `404` for unlisted files and load failure for a missing, symlinked, wrong-size, or wrong-hash artifact.

- [ ] **Step 2: Run the server catalog tests and verify RED**

Run: `go test ./internal/server -run 'TestUpdateCatalog' -count=1`

Expected: FAIL because catalog types and routes do not exist.

- [ ] **Step 3: Implement immutable catalog loading and handlers**

```go
type UpdateCatalog struct {
    directory string
    rawManifest []byte
    signature []byte
    manifest update.Manifest
    minimum update.Version
    files map[string]update.Artifact
}

type RoomsOption func(*Server)
func WithUpdateCatalog(catalog *UpdateCatalog) RoomsOption {
    return func(service *Server) { service.updates = catalog }
}
```

Use `os.Lstat` to reject symlinks and non-regular files, hash each artifact at load, and register only exact manifest, signature, and `/artifacts/<base-name>` routes. Set `Content-Length`, `Content-Type: application/octet-stream`, `X-Content-Type-Options: nosniff`, and `Cache-Control: no-store` for manifests; artifacts may use immutable caching keyed by versioned filename.

- [ ] **Step 4: Run server catalog and existing server tests**

Run: `go test ./internal/server -count=1`

Expected: PASS, including callers that still use `NewRooms(repository)`.

- [ ] **Step 5: Commit**

```bash
git add internal/server/update_catalog.go internal/server/update_catalog_test.go internal/server/server.go internal/server/rooms.go
git commit -m "feat:服务端下发签名更新清单与客户端制品"
```

### Task 3: WebSocket Client Compatibility Enforcement

**Files:**
- Modify: `internal/protocol/protocol.go`
- Modify: `internal/protocol/protocol_test.go`
- Modify: `internal/server/server.go`
- Create: `internal/server/version_test.go`
- Modify: `internal/client/client.go`
- Create: `internal/client/version_test.go`

**Interfaces:**
- Consumes: `UpdateCatalog.MinimumVersion()` from Task 2.
- Produces: `protocol.Join.ClientVersion`, `ClientOS`, and `ClientArch` JSON fields.
- Produces: `protocol.Failure.MinimumVersion` for `upgrade_required` responses.
- Produces: `client.Info{Version, OS, Arch string}` and `client.NewWithInfo(address string, info Info) *Client`.

- [ ] **Step 1: Write failing direct-join compatibility tests**

```go
failure := service.join(makeSession(), protocol.Join{
    Nickname: "old", AccessKey: "room", ClientVersion: "0.3.1", ClientOS: "windows", ClientArch: "amd64",
})
if failure.Code != "upgrade_required" || failure.MinimumVersion != "0.3.2" { t.Fatal(failure) }
```

Cover missing and malformed versions, unsupported platform, exact minimum, latest, and newer-than-latest. Verify minimum `0.0.0` permits a missing version for staged migration.

- [ ] **Step 2: Run server compatibility tests and verify RED**

Run: `go test ./internal/server -run 'TestClientVersion' -count=1`

Expected: FAIL because join metadata and enforcement are absent.

- [ ] **Step 3: Add protocol fields and enforce before room access**

```go
type Join struct {
    AccessKey string `json:"access_key"`
    Nickname string `json:"nickname"`
    InstanceID string `json:"instance_id,omitempty"`
    AfterID int64 `json:"after_id,omitempty"`
    ClientVersion string `json:"client_version,omitempty"`
    ClientOS string `json:"client_os,omitempty"`
    ClientArch string `json:"client_arch,omitempty"`
}
type Failure struct {
    Code string `json:"code"`
    Message string `json:"message"`
    MinimumVersion string `json:"minimum_version,omitempty"`
}
```

Call a focused `service.checkClient(join)` before access-key validation and storage access. Treat only `windows/amd64` and `linux/amd64` as supported when enforcement is enabled.

- [ ] **Step 4: Write failing client metadata and terminal-error tests**

Use an `httptest` WebSocket server to capture the first join frame and return `upgrade_required`. Assert `NewWithInfo` sends all three fields and `Run` emits one terminal event with `State == "upgrade_required"` and does not retry.

- [ ] **Step 5: Run client tests and verify RED**

Run: `go test ./internal/client -run 'TestClientVersion|TestUpgradeRequired' -count=1`

Expected: FAIL because client metadata is not represented.

- [ ] **Step 6: Implement client metadata with backward-compatible constructor**

```go
type Info struct { Version, OS, Arch string }
func New(address string) *Client { return NewWithInfo(address, Info{}) }
func NewWithInfo(address string, info Info) *Client {
    return &Client{address: address, info: info, events: make(chan Event, 128), retryMin: time.Second}
}
```

Populate the join frame from `Client.info`; classify `upgrade_required` and `unsupported_client` with the existing non-retry terminal join errors.

- [ ] **Step 7: Run protocol, server, and client tests and commit**

Run: `go test ./internal/protocol ./internal/server ./internal/client -count=1`

Expected: PASS.

```bash
git add internal/protocol internal/server internal/client
git commit -m "feat:服务端强制校验客户端兼容版本"
```

### Task 4: Managed Layout and Stable `spacechat` Launcher

**Files:**
- Create: `internal/update/layout.go`
- Create: `internal/update/layout_test.go`
- Create: `internal/launcher/launcher.go`
- Create: `internal/launcher/launcher_test.go`
- Create: `cmd/spacechat/main.go`
- Create: `cmd/spacechat/main_test.go`

**Interfaces:**
- Consumes: strict `update.Version` from Task 1.
- Produces: `update.LayoutFor(goos, home string, getenv func(string) string) (Layout, error)`.
- Produces: `Layout.ClientPath(version Version, goos string) (string, error)`, `ReadCurrent(Layout)`, and `WriteCurrent(Layout, Version)`.
- Produces: `launcher.Run(layout update.Layout, args []string, streams Streams, runner CommandRunner) error`.

- [ ] **Step 1: Write failing cross-platform layout and atomic-pointer tests**

Assert the exact Windows and Linux paths from the spec, rejection of an invalid `current`, and replacement of `current` without a partially written value. Include a test that `ClientPath` cannot escape `versions`.

- [ ] **Step 2: Run layout tests and verify RED**

Run: `go test ./internal/update -run 'TestLayout|TestCurrent' -count=1`

Expected: FAIL because layout functions do not exist.

- [ ] **Step 3: Implement layout and atomic current writes**

```go
type Layout struct {
    Root, Bin, Config, Current, Versions, Temp, Lock string
}
func (layout Layout) ClientPath(version Version, goos string) (string, error) {
    name := "spacechat-client"
    if goos == "windows" { name += ".exe" }
    return filepath.Join(layout.Versions, version.String(), name), nil
}
```

Write `current.new` with mode `0600`, close and rename it in the same directory; create parent directories with `0700` on Unix-compatible filesystems.

- [ ] **Step 4: Write failing launcher tests**

Create a fake executable in the managed version directory and inject a command runner. Assert argument, stdin, stdout, stderr, working directory, and exit-code forwarding. Assert missing or non-regular managed clients fail without executing anything.

- [ ] **Step 5: Run launcher tests and verify RED**

Run: `go test ./internal/launcher ./cmd/spacechat -count=1`

Expected: FAIL because the launcher does not exist.

- [ ] **Step 6: Implement the network-free launcher**

```go
type Streams struct { Stdin io.Reader; Stdout, Stderr io.Writer }
type CommandRunner func(path string, args []string, streams Streams) error
func Run(layout update.Layout, args []string, streams Streams, runner CommandRunner) error
```

The production runner uses `exec.Command`, attaches `os.Stdin`, `os.Stdout`, and `os.Stderr`, and returns the child's exit status. `cmd/spacechat/main.go` resolves the current user's layout and passes `os.Args[1:]` unchanged.

- [ ] **Step 7: Run launcher tests and commit**

Run: `go test ./internal/update ./internal/launcher ./cmd/spacechat -count=1`

Expected: PASS.

```bash
git add internal/update/layout.go internal/update/layout_test.go internal/launcher cmd/spacechat
git commit -m "feat:新增跨平台spacechat稳定启动入口"
```

### Task 5: Secure Fetch, Transactional Install, Lock, and Cleanup

**Files:**
- Create: `internal/update/remote.go`
- Create: `internal/update/remote_test.go`
- Create: `internal/update/install.go`
- Create: `internal/update/install_test.go`
- Create: `internal/update/lock.go`
- Create: `internal/update/lock_test.go`

**Interfaces:**
- Consumes: manifest, artifact, version, and layout types from Tasks 1 and 4.
- Produces: `UpdateURL(serverAddress, endpoint string) (*url.URL, error)`.
- Produces: `Remote.Check(ctx, serverAddress, current, goos, goarch) (Check, error)` and `Remote.Download(ctx, serverAddress string, artifact Artifact, destination string, progress func(received, total int64)) error`.
- Produces: `Installer.Install(ctx, Check) (InstallResult, error)` and `Installer.Rollback(InstallResult) error`.
- Produces: `Acquire(layout Layout, now time.Time) (*Lock, error)` and `Cleanup(layout Layout, current Version) error`.

- [ ] **Step 1: Write failing URL and signed-check tests**

Assert `ws://host:18081/ws` maps to `http://host:18081/updates/v1/manifest`, `wss` maps to `https`, credentials/fragments/non-WebSocket schemes are rejected, redirects are rejected, and a signed test server returns the correct optional/required decision and platform artifact.

- [ ] **Step 2: Run remote check tests and verify RED**

Run: `go test ./internal/update -run 'TestUpdateURL|TestRemoteCheck' -count=1`

Expected: FAIL because remote operations do not exist.

- [ ] **Step 3: Implement bounded same-origin fetches**

```go
type Remote struct {
    Client *http.Client
    PublicKey ed25519.PublicKey
}
type Check struct {
    Manifest Manifest
    Artifact Artifact
    Current, Latest, Minimum Version
    Decision Decision
}
```

Use request contexts, a client with `CheckRedirect` returning `http.ErrUseLastResponse`, exact `200` status, a 1 MiB manifest limit, exact 64-byte signature, and artifact URLs constructed only from the selected server origin plus the validated filename.

- [ ] **Step 4: Write failing download/install/lock tests**

Test exact size and digest, short and oversized bodies, interrupted downloads, progress callbacks, destination cleanup, executable mode on Linux, self-check failure, atomic current switch, a second concurrent lock, stale lock recovery after ten minutes, and cleanup retaining current plus one prior version.

- [ ] **Step 5: Run installer tests and verify RED**

Run: `go test ./internal/update -run 'TestDownload|TestInstall|TestLock|TestCleanup' -count=1`

Expected: FAIL because installer and lock operations do not exist.

- [ ] **Step 6: Implement transactional installation**

```go
type Installer struct {
    Layout Layout
    Remote Remote
    GOOS string
    SelfCheck func(context.Context, string) error
}
type InstallResult struct {
    Path string
    Previous Version
    Target Version
}
```

Create a unique file under `tmp`, stream through `sha256.New()` and `io.MultiWriter`, enforce `artifact.Size` and `MaxArtifactSize`, sync and close before rename, create a new version directory, set mode `0700` for directories and `0755` for Linux executables, run `<new-client> --self-check`, then call `WriteCurrent`. On every error remove only paths created for the target version and leave `current` untouched.

Implement the update lock as an `O_CREATE|O_EXCL` metadata file containing PID and RFC3339 timestamp. A lock older than ten minutes may be removed once and reacquired; live fresh locks are never removed.

- [ ] **Step 7: Run update package tests and commit**

Run: `go test ./internal/update -count=1`

Expected: PASS.

```bash
git add internal/update
git commit -m "feat:实现客户端安全下载与事务更新"
```

### Task 6: Update Prompt and Orchestration

**Files:**
- Create: `internal/update/runner.go`
- Create: `internal/update/runner_test.go`
- Create: `internal/updateui/prompt.go`
- Create: `internal/updateui/prompt_test.go`

**Interfaces:**
- Consumes: `Remote`, `Installer`, `Check`, and decisions from Task 5.
- Produces: `update.UI` with `Choose(Prompt) Action`, `Progress(received, total int64)`, and `ShowError(error)`.
- Produces: `Runner.Run(ctx, Request) (Outcome, error)` where outcomes are `Continue`, `Relaunched`, and `Exit`.
- Produces: `updateui.New(in io.Reader, out io.Writer) *Console` implementing `update.UI`.

- [ ] **Step 1: Write failing prompt tests**

Feed `u`, `s`, `r`, and `e` through a buffered reader. Assert optional prompts accept only Update/Skip, required prompts accept only Update/Exit, required retry prompts accept only Retry/Exit, invalid input repeats, and progress output never divides by zero.

- [ ] **Step 2: Run prompt tests and verify RED**

Run: `go test ./internal/updateui -count=1`

Expected: FAIL because update UI does not exist.

- [ ] **Step 3: Implement line-oriented pre-TUI interaction**

```go
type Action uint8
const (ActionUpdate Action = iota; ActionSkip; ActionRetry; ActionExit)
type Prompt struct {
    Current, Latest Version
    Notes string
    Decision Decision
    Retry bool
}
type UI interface {
    Choose(Prompt) Action
    Progress(received, total int64)
    ShowError(error)
}
```

Keep these types in `internal/update/runner.go`. Implement `updateui.Console.Choose(update.Prompt) update.Action` plus the progress and error methods. Render concise Chinese text before Bubble Tea starts. Flush progress by rewriting one line when attached to a terminal and by emitting bounded milestone lines for buffered/non-terminal tests.

- [ ] **Step 4: Write failing runner policy tests**

Inject fake remote, installer, UI, and launcher functions. Cover current/ahead continuation, optional skip, optional install failure continuation, required exit, required failure retry, required failure exit, successful install and relaunch, check timeout continuation, and signature error continuation without executing content.

- [ ] **Step 5: Run runner tests and verify RED**

Run: `go test ./internal/update -run TestRunner -count=1`

Expected: FAIL because orchestration does not exist.

- [ ] **Step 6: Implement policy runner**

```go
type Outcome uint8
const (Continue Outcome = iota; Relaunched; Exit)
type Request struct { Server string; Current Version; GOOS, GOARCH string; Args []string }
type Runner struct {
    Remote Remote
    Installer Installer
    UI UI
    Launch func(path string, args []string) error
}
```

Loop only for a required update after `ActionRetry`; optional failures return `Continue`. A successful install starts the returned managed client with the original arguments and returns `Relaunched` so the old process exits before starting Bubble Tea. If process start fails, call `Installer.Rollback(result)` before applying the optional/required failure policy.

- [ ] **Step 7: Run update and UI tests and commit**

Run: `go test ./internal/update ./internal/updateui -count=1`

Expected: PASS.

```bash
git add internal/update/runner.go internal/update/runner_test.go internal/updateui
git commit -m "feat:新增客户端更新提示与失败回退流程"
```

### Task 7: Integrate Update Preflight and Versioned Client Identity

**Files:**
- Create: `internal/clientconfig/config.go`
- Create: `internal/clientconfig/config_test.go`
- Modify: `cmd/xchat/main.go`
- Modify: `cmd/xchat/main_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/room_switch.go`
- Modify: `internal/tui/model_test.go`
- Modify: `internal/tui/room_switch_test.go`

**Interfaces:**
- Consumes: updater runner, UI, layout, and `client.Info`.
- Produces: `clientconfig.Load(path, fallback string) (Config, error)` with `Config.Server`.
- Produces: `tui.NewWithClientInfo(address string, info client.Info) *Model` while preserving `tui.New(address)` for tests and compatibility.
- Produces: `Model.UpgradeRequired() bool` so `cmd/xchat` can return to preflight after a race with a server minimum-version change.

- [ ] **Step 1: Write failing config and command-mode tests**

Assert a strict JSON config with one `server` field, fallback to loopback when the managed config is missing, command-line `--server` precedence, `--version`, and hidden `--self-check` success only when version and embedded public key are valid.

- [ ] **Step 2: Run command tests and verify RED**

Run: `go test ./internal/clientconfig ./cmd/xchat -count=1`

Expected: FAIL because config and update-aware command flow do not exist.

- [ ] **Step 3: Implement config and testable command runner**

Refactor `main` into `run(args []string, stdin io.Reader, stdout, stderr io.Writer) int`. Keep source fallbacks:

```go
var defaultServer = "ws://127.0.0.1:18081/ws"
var version = "dev"
var updatePublicKey = ""
```

Decode the build-injected public key from base64. Development binaries with `version == "dev"` or no public key skip online installation but still run chat with their explicit metadata. Managed release binaries run preflight before constructing the TUI.

- [ ] **Step 4: Write failing TUI client-info and upgrade-race tests**

Assert both initial login and F2 candidate connections receive the same `client.Info`. Feed a terminal `upgrade_required` client event and assert the Bubble Tea model quits with `UpgradeRequired() == true` without sending or showing room history.

- [ ] **Step 5: Run TUI tests and verify RED**

Run: `go test ./internal/tui -run 'TestClientInfo|TestUpgradeRequired' -count=1`

Expected: FAIL because TUI always calls `client.New` and has no upgrade exit state.

- [ ] **Step 6: Integrate metadata and update restart loop**

Add `clientInfo client.Info` and a small client factory to `Model`; `New` delegates to `NewWithClientInfo`. Preserve metadata when creating the room-switch candidate. On `upgrade_required`, mark the model, cancel networking, and return `tea.Quit`. In `cmd/xchat`, repeat update preflight when the model exits for this reason; all other exits retain current behavior.

- [ ] **Step 7: Run command, TUI, client tests and commit**

Run: `go test ./cmd/xchat ./internal/clientconfig ./internal/client ./internal/tui -count=1`

Expected: PASS.

```bash
git add cmd/xchat internal/clientconfig internal/tui
git commit -m "feat:客户端启动时检查更新并上报版本"
```

### Task 8: Release Signing Tool and Cross-Platform Build Outputs

**Files:**
- Create: `cmd/spacechat-release/main.go`
- Create: `cmd/spacechat-release/main_test.go`
- Modify: `scripts/build.ps1`
- Modify: `scripts/build-client.ps1`
- Create: `deploy/client/install.ps1`
- Create: `deploy/client/install.sh`
- Create: `deploy/test_client_install.py`

**Interfaces:**
- Consumes: manifest validation rules from Task 1.
- Produces: `spacechat-release public-key -private-key <base64-key-file>`.
- Produces: `spacechat-release manifest -version <x.y.z> -minimum <x.y.z> -private-key <file> -windows <file> -linux <file> -out <dir>`.
- Produces: user-level installers accepting the server WebSocket URL and initial version directory.

- [ ] **Step 1: Write failing release-tool tests**

Use an ephemeral Ed25519 private key. Assert `public-key` prints the matching base64 public key; `manifest` copies both artifacts, writes exact size/digests, creates a raw 64-byte signature, and its output passes `update.VerifyManifest`. Assert malformed keys and invalid version relationships fail without a partial output directory.

- [ ] **Step 2: Run release-tool tests and verify RED**

Run: `go test ./cmd/spacechat-release -count=1`

Expected: FAIL because the command does not exist.

- [ ] **Step 3: Implement deterministic release metadata**

Split CLI parsing from `run(args, stdout, stderr) int`. Marshal the manifest with `json.MarshalIndent` plus one trailing newline, sign those exact bytes, write artifacts first, and atomically rename the completed output directory. Read a base64 32-byte seed or 64-byte Ed25519 private key from a regular permission-restricted file; never print it.

- [ ] **Step 4: Write failing installer-script assertions**

In `deploy/test_client_install.py`, assert Windows installs under `%LOCALAPPDATA%\SpaceChat`, Linux under XDG/user directories, both write strict `config.json` and `current`, the Windows installer updates user PATH rather than machine PATH, and neither installer embeds a production server address.

- [ ] **Step 5: Run deployment script tests and verify RED**

Run: `python3 -m unittest deploy.test_client_install -v`

Expected: FAIL because installers do not exist.

- [ ] **Step 6: Implement build and installation scripts**

Change release version default to `0.4.0`; add mandatory `-SigningKey` and optional `-MinimumVersion` defaulting to `0.0.0`. Derive the public key before compiling clients and inject it with:

```powershell
$clientFlags = "-s -w -X main.defaultServer=$Server -X main.version=$Version -X main.updatePublicKey=$publicKey"
```

Build stable launcher and versioned client for Windows amd64 and Linux amd64, then build the Linux server. Run `spacechat-release manifest` and package the signed update directory with the server. Installers validate the server URL, copy launcher/client, write the pointer/config atomically, and add only the user-level executable directory to PATH.

- [ ] **Step 7: Run tool and script tests and commit**

Run: `go test ./cmd/spacechat-release -count=1 && python3 -m unittest deploy.test_client_install -v`

Expected: PASS.

```bash
git add cmd/spacechat-release scripts deploy/client deploy/test_client_install.py
git commit -m "feat:新增客户端签名发布与首次安装工具"
```

### Task 9: Wire Update Catalog into Server Deployment

**Files:**
- Modify: `cmd/xchat-server/main.go`
- Create: `cmd/xchat-server/main_test.go`
- Modify: `deploy/rooms/install.sh`
- Modify: `deploy/rooms/xchat-rooms.service`
- Modify: `deploy/test_rooms_deploy.py`
- Modify: `README.md`

**Interfaces:**
- Consumes: `server.LoadUpdateCatalog` and `server.WithUpdateCatalog`.
- Produces: server flags `-update-dir` and `-update-public-key-file`; both omitted means staged migration mode without catalog or version enforcement, both present enables them, and specifying only one is a startup error.

- [ ] **Step 1: Write failing server flag tests**

Refactor server startup parsing into a testable config function. Assert both update flags are paired, the public key file must contain one base64 32-byte Ed25519 public key, and a valid catalog is passed to `NewRooms`.

- [ ] **Step 2: Run server command tests and verify RED**

Run: `go test ./cmd/xchat-server -count=1`

Expected: FAIL because update flags and parsing do not exist.

- [ ] **Step 3: Implement server catalog wiring**

Add flags with empty development defaults, load and decode the regular public-key file, call `server.LoadUpdateCatalog`, and pass `server.WithUpdateCatalog(catalog)`. Log latest/minimum versions without logging manifest signatures or filesystem contents.

- [ ] **Step 4: Extend failing deployment assertions**

Require the installer to copy the signed update directory to `/opt/xchat-rooms/updates`, copy the public key to `/etc/xchat-rooms/update-public.key` with `0644`, reject symlinks, and add both flags to the service unit. Require an atomic `updates.new` validation/swap so a failed deployment retains the previous catalog.

- [ ] **Step 5: Run deployment tests and verify RED**

Run: `python3 -m unittest deploy.test_rooms_deploy -v`

Expected: FAIL until installer and unit include update assets.

- [ ] **Step 6: Implement deployment and operator documentation**

Update `README.md` with first installation, `spacechat`, `--server`, optional/required update behavior, release signing, minimum-version rollout, failure recovery, and rollback instructions. Keep all example addresses non-production placeholders.

- [ ] **Step 7: Run server/deployment tests and commit**

Run: `go test ./cmd/xchat-server ./internal/server -count=1 && python3 -m unittest deploy.test_rooms_deploy -v`

Expected: PASS.

```bash
git add cmd/xchat-server deploy/rooms deploy/test_rooms_deploy.py README.md
git commit -m "feat:部署服务端客户端更新目录与版本策略"
```

### Task 10: End-to-End Update Verification and Regression

**Files:**
- Create: `internal/update/integration_test.go`
- Create: `docs/verification-client-online-update-2026-09-21.md`
- Modify: tests only where a proven platform-specific gap is exposed.

**Interfaces:**
- Consumes: all production interfaces from Tasks 1–9.
- Produces: an automated end-to-end proof that an old managed client can optionally update, is forced when below minimum, preserves the old version on corruption, and remains blocked by the WebSocket server when incompatible.

- [ ] **Step 1: Write the end-to-end update test**

Use a temporary installation root, ephemeral signing key, signed manifest, `httptest` server, fake executable self-check, and real filesystem operations. Exercise:

```text
0.3.2 -> optional 0.4.0 -> Skip -> old pointer unchanged
0.3.2 -> optional 0.4.0 -> Update -> self-check -> pointer 0.4.0
0.3.1 -> required 0.4.0 -> corrupt body -> pointer 0.3.1 and no Continue
0.3.1 -> WebSocket join with minimum 0.3.2 -> upgrade_required before history
```

- [ ] **Step 2: Run the integration test and verify it exercises real boundaries**

Run: `go test ./internal/update -run TestManagedClientUpdateEndToEnd -count=1 -v`

Expected: PASS only through the real manifest verifier, HTTP downloader, filesystem installer, self-check hook, atomic pointer, and server compatibility decision; no direct field mutation may replace those boundaries.

- [ ] **Step 3: Run formatting and focused race verification**

Run: `gofmt -w cmd internal`

Run: `go test -race ./internal/update ./internal/server ./internal/client ./internal/tui -count=1`

Expected: PASS with no race reports.

- [ ] **Step 4: Run the full Go verification**

Run: `go test ./... -count=1`

Run: `go vet ./...`

Expected: both commands exit `0`.

- [ ] **Step 5: Run deployment-script verification**

Run: `python3 -m unittest discover -s deploy -p 'test_*.py'`

Expected: all deployment tests pass.

- [ ] **Step 6: Verify cross-compilation without publishing artifacts**

Use a temporary directory outside the repository:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/spacechat-windows-amd64.exe ./cmd/spacechat
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/spacechat-client-windows-amd64.exe ./cmd/xchat
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/spacechat-linux-amd64 ./cmd/spacechat
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/spacechat-client-linux-amd64 ./cmd/xchat
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/spacechat-server-linux-amd64 ./cmd/xchat-server
```

Expected: all five builds exit `0`; no `dist` or generated artifact is added to Git.

- [ ] **Step 7: Record fresh verification evidence**

Create `docs/verification-client-online-update-2026-09-21.md` with the exact commands, timestamps, exit codes, test counts, supported platforms, and any intentionally unrun environment-dependent Windows pseudo-terminal checks. Do not claim an unrun check passed.

- [ ] **Step 8: Check repository hygiene and commit**

Run: `git diff --check && git status --short`

Expected: only intended source, tests, scripts, README, plan, and verification document are changed; no private key, binary, database, build directory, or local IDE file is present.

```bash
git add internal/update/integration_test.go docs/verification-client-online-update-2026-09-21.md
git commit -m "style:补充客户端在线更新端到端验收"
```

- [ ] **Step 9: Inspect every pending push commit without pushing**

Run: `git log --reverse --format='%h %s' origin/main..HEAD`

Expected: list the inherited noncompliant `8d5333a doc:需求收集0918.md` separately from the new compliant commits. Do not push, amend, rebase, or reset until the user explicitly decides how to handle that inherited history.
