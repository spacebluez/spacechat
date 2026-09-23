# SpaceChat Docker Compose 一键部署 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让用户从 Git 获取仓库后，通过一条 `docker compose up -d --build` 命令启动带客户端首装、在线更新、持久化存储和每日清理的 SpaceChat 服务。

**Architecture:** Compose 编排一次性 `artifacts` 构建服务、长期运行的 `server` 和定时 `cleanup`。制品构建器使用默认公开或用户挂载的 Ed25519 私钥交叉编译客户端并原子发布到共享卷；服务端只读取制品，独立生成并持久化数据库密钥，首装脚本按请求来源动态写入客户端地址。

**Tech Stack:** Go 1.26、Python 3、POSIX shell、Docker 多阶段构建、Docker Compose v2、SQLite、Ed25519。

**Spec:** `docs/superpowers/specs/2026-09-22-docker-compose-deployment-design.md`

## 实施进度（2026-09-23）

- Tasks 1–6 已在当前分支实现，最近提交为 `22d1fb2`（制品发布事务恢复）。
- Task 7 已补齐 Dockerfile、构建上下文排除、TLS 入口、健康检查和镜像行为验收脚本；三个真实镜像的构建及行为检查通过。
- Task 8 已补齐 Compose、三项真实 Compose 配置解析测试、隔离的 WS/WSS 冒烟脚本；两种模式均通过实际客户端首装、重启保留密钥及手动清理验证。
- Task 9 已补齐 README、`.env.example` 和独立验收记录；Linux 上 18 个 Go 包、37 项部署测试及 `go vet` 全部通过。
- 复查 Task 5 时修复了秒数向下取整引起的提前清理与一秒后重试，并补充受控时钟回归测试。
- 已安装并验证 WSL2 Ubuntu、Docker Engine 29.1.3 与 Compose 2.40.3。网络受限时使用官方基础镜像的 ECR 下载入口与校验过的本地依赖缓存；构建允许覆盖下载源，运行时仍断网生成制品。
- 自动化容器验收已完成；独立桌面客户端聊天、真实版本升级、含历史消息的备份恢复和实际跨午夜运行等人工场景仍待验收，具体边界见验证记录。

本节记录实际实施状态；下方保留原分步执行模板。详细命令、已知失败及剩余验收见 `docs/verification-docker-compose-2026-09-23.md`。

## Global Constraints

- 默认命令必须是 `docker compose up -d --build`，不能要求宿主机安装 Go、PowerShell、Python 或 systemd。
- 默认监听宿主机 `18081`，使用 `ws://`，只适用于可信内网；公网部署文档必须要求 WSS、公开地址和访问控制。
- 客户端制品仅支持 Windows amd64 与 Linux amd64；服务端镜像运行于 Linux。
- 默认更新签名 seed 必须公开提交，并明确不代表发布者身份；程序不得强制拒绝默认 seed。
- 自定义更新私钥只挂载给 `artifacts`，不得进入 `server`、`cleanup`、运行镜像层或制品卷。
- 数据库密钥首次启动独立随机生成；已有数据库缺失密钥时必须失败，绝不生成替代密钥。
- SQLite 数据库与其密钥共同保存在 `data` 卷；签名制品和管理 socket 分别使用独立卷。
- 每日清理发生在 `Asia/Shanghai` 00:00，停机错过后不补跑，失败不立即重试。
- 现有 schema 1 固定首装地址、PowerShell 发布脚本和 systemd 部署必须保持兼容。
- 所有实现遵循测试先行；提交标题只使用仓库允许的 `feat:`、`bug:`、`refactor:`、`style:`、`docs:` 类型。

## File Structure

- `internal/update/installer_manifest.go`：schema 1 固定地址和 schema 2 请求地址模式的严格验证。
- `cmd/spacechat-release/main.go`：生成两种首装清单。
- `internal/server/installer_catalog.go`：从请求或显式公开 URL 解析首装下载地址和 WebSocket 地址。
- `cmd/xchat-server/main.go`：接收公开 URL 和容器密钥初始化参数。
- `internal/clientconfig/config.go`：保存只对安装配置地址生效的 `allow_insecure`。
- `cmd/xchat/main.go`：合并安装配置与命令行的明文授权，不扩大命令行地址权限。
- `internal/securestore/key.go`：数据库密钥安全首次创建。
- `deploy/clear_history.py`：一次性清理和上海午夜调度。
- `deploy/docker/build-artifacts.sh`：跨平台客户端、首装包和更新目录构建及原子发布。
- `deploy/docker/server-entrypoint.sh`：TLS 成对检测和服务端参数组装。
- `deploy/docker/healthcheck.sh`：WS/WSS 本机健康检查。
- `Dockerfile`：`artifacts`、`server`、`cleanup` 多阶段镜像。
- `compose.yaml`：服务依赖、端口、只读挂载、健康检查与命名卷。
- `deploy/test_docker_*.py`、`deploy/docker/smoke-test.sh`：静态配置与真实 Compose 验收。
- `README.md`、`.env.example`：零配置、密钥、TLS、公网、升级和备份说明。

---

### Task 1: 动态首装清单协议

**Files:**
- Modify: `internal/update/installer_manifest.go`
- Modify: `internal/update/installer_manifest_test.go`
- Modify: `cmd/spacechat-release/main.go`
- Modify: `cmd/spacechat-release/main_test.go`

**Interfaces:**
- Produces: `const InstallerServerModeRequest = "request"`。
- Produces: `InstallerManifest.ServerMode string`，JSON 字段为可省略的 `server_mode`。
- Produces: `createInstallerCatalog(versionText, server, serverMode, keyPath, windowsPath, linuxPath, outputPath string) error`。
- Produces: `spacechat-release installers -server-mode request`；`-server` 与 `-server-mode` 必须且只能提供一个。

- [ ] **Step 1: 为 schema 2 和互斥字段编写失败测试**

在 `internal/update/installer_manifest_test.go` 增加：

```go
func TestVerifyInstallerManifestAcceptsRequestAddressMode(t *testing.T) {
	manifest := validInstallerManifest()
	manifest.Schema = 2
	manifest.Server = ""
	manifest.ServerMode = InstallerServerModeRequest
	raw, signature, publicKey := signInstallerManifest(t, manifest)
	verified, err := VerifyInstallerManifest(raw, signature, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Server != "" || verified.ServerMode != InstallerServerModeRequest {
		t.Fatalf("verified manifest = %+v", verified)
	}
}

func TestVerifyInstallerManifestRejectsMixedAddressModes(t *testing.T) {
	for name, mutate := range map[string]func(*InstallerManifest){
		"schema one with mode": func(m *InstallerManifest) { m.ServerMode = InstallerServerModeRequest },
		"schema two with server": func(m *InstallerManifest) { m.Schema = 2; m.ServerMode = InstallerServerModeRequest },
		"schema two without mode": func(m *InstallerManifest) { m.Schema = 2; m.Server = "" },
		"unknown mode": func(m *InstallerManifest) { m.Schema = 2; m.Server = ""; m.ServerMode = "proxy" },
	} {
		t.Run(name, func(t *testing.T) {
			manifest := validInstallerManifest()
			mutate(&manifest)
			raw, signature, publicKey := signInstallerManifest(t, manifest)
			if _, err := VerifyInstallerManifest(raw, signature, publicKey); err == nil {
				t.Fatal("invalid address mode accepted")
			}
		})
	}
}
```

在 `cmd/spacechat-release/main_test.go` 增加动态模式成功测试和同时/都不提供两个参数的失败测试。

- [ ] **Step 2: 运行目标测试并确认失败**

Run:

```sh
env GOCACHE=/tmp/spacechat-go-build-cache go test ./internal/update ./cmd/spacechat-release
```

Expected: FAIL，提示 `ServerMode` 或 `InstallerServerModeRequest` 未定义。

- [ ] **Step 3: 实现严格的双 schema 验证**

将清单结构扩展为：

```go
const InstallerServerModeRequest = "request"

type InstallerManifest struct {
	Schema     int                 `json:"schema"`
	Version    string              `json:"version"`
	Server     string              `json:"server,omitempty"`
	ServerMode string              `json:"server_mode,omitempty"`
	Packages   map[string]Artifact `json:"packages"`
}
```

在 `validate` 中先按 schema 验证地址字段，再执行现有平台、文件名、大小和摘要验证：

```go
switch manifest.Schema {
case 1:
	if manifest.ServerMode != "" {
		return errors.New("schema 1 installer manifest cannot set server_mode")
	}
	server, err := url.Parse(manifest.Server)
	if err != nil || server.Host == "" || (server.Scheme != "ws" && server.Scheme != "wss") || server.User != nil || server.Fragment != "" {
		return errors.New("invalid installer server URL")
	}
case 2:
	if manifest.Server != "" || manifest.ServerMode != InstallerServerModeRequest {
		return errors.New("schema 2 installer manifest requires request server mode")
	}
default:
	return errors.New("unsupported installer manifest schema")
}
```

- [ ] **Step 4: 扩展发布工具的动态模式参数**

`installers` 子命令增加 `-server-mode`，并使用以下互斥校验：

```go
server := set.String("server", "", "fixed WebSocket server address")
serverMode := set.String("server-mode", "", "installer server mode; request derives it from the download request")
if (*server == "") == (*serverMode == "") {
	fmt.Fprintln(stderr, "spacechat-release: installers requires exactly one of -server or -server-mode")
	return 2
}
if *serverMode != "" && *serverMode != update.InstallerServerModeRequest {
	fmt.Fprintln(stderr, "spacechat-release: unsupported installer server mode")
	return 2
}
```

`createInstallerCatalog` 根据输入生成 schema 1 或 schema 2，但共享现有包复制、摘要和 Ed25519 签名流程。

- [ ] **Step 5: 运行测试确认通过**

Run:

```sh
env GOCACHE=/tmp/spacechat-go-build-cache go test ./internal/update ./cmd/spacechat-release
```

Expected: PASS。

- [ ] **Step 6: 提交**

```sh
git add internal/update/installer_manifest.go internal/update/installer_manifest_test.go cmd/spacechat-release/main.go cmd/spacechat-release/main_test.go
git commit -m "feat:支持动态首装地址清单"
```

### Task 2: 根据请求安全生成首装地址

**Files:**
- Modify: `internal/server/installer_catalog.go`
- Modify: `internal/server/installer_catalog_test.go`
- Modify: `cmd/xchat-server/main.go`
- Modify: `cmd/xchat-server/main_test.go`

**Interfaces:**
- Consumes: `InstallerManifest.ServerMode` 和 `InstallerServerModeRequest`。
- Produces: `func (catalog *InstallerCatalog) SetPublicURL(raw string) error`。
- Produces: `serverConfig.publicURL string`，命令行参数 `-public-url`，默认读取 `SPACECHAT_PUBLIC_URL`。
- Behavior: schema 1 始终使用签名固定地址；schema 2 优先显式公开 URL，否则只使用 `request.TLS` 与 `request.Host`，忽略转发头。

- [ ] **Step 1: 编写 HTTP、HTTPS、覆盖和恶意 Host 测试**

给 fixture 增加生成 schema 2 的参数，并在 `internal/server/installer_catalog_test.go` 增加表格测试：

```go
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
			if err := catalog.SetPublicURL(test.publicURL); err != nil { t.Fatal(err) }
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
```

另测 `request.Host = "user@host"`、含空白/路径分隔符的 Host 返回 400；无效 `SPACECHAT_PUBLIC_URL` 使目录加载失败；schema 1 继续忽略请求 Host。

- [ ] **Step 2: 运行测试并确认失败**

Run:

```sh
env GOCACHE=/tmp/spacechat-go-build-cache go test ./internal/server ./cmd/xchat-server
```

Expected: FAIL，动态 fixture 仍尝试把空 `Server` 解析成固定 URL，且 `SetPublicURL` 不存在。

- [ ] **Step 3: 实现地址解析边界**

在目录中保存可选覆盖 URL，并实现一个集中解析函数：

```go
func installerRequestAddresses(request *http.Request, override *url.URL) (origin, server string, err error) {
	websocketScheme, httpScheme, host := "ws", "http", request.Host
	if override != nil {
		websocketScheme, host = override.Scheme, override.Host
		if websocketScheme == "wss" { httpScheme = "https" }
	} else if request.TLS != nil {
		websocketScheme, httpScheme = "wss", "https"
	}
	if err := validateInstallerHost(host); err != nil { return "", "", err }
	return (&url.URL{Scheme: httpScheme, Host: host}).String(),
		(&url.URL{Scheme: websocketScheme, Host: host, Path: "/ws"}).String(), nil
}
```

`SetPublicURL` 只接受无用户信息、无 fragment、Host 非空、scheme 为 `ws`/`wss`、path 精确为 `/ws` 的 URL。动态 handler 解析失败返回 `http.StatusBadRequest`，固定模式保持现有签名地址行为。

- [ ] **Step 4: 把公开 URL 接入服务端配置**

在 `serverConfig` 和 `parseConfig` 增加：

```go
publicURL string
set.StringVar(&config.publicURL, "public-url", os.Getenv("SPACECHAT_PUBLIC_URL"), "Public ws[s] URL used by dynamic installer scripts")
```

`loadInstallerOptions` 在目录加载后调用 `catalog.SetPublicURL(config.publicURL)`，错误包装为 `installer public URL`。更新 `cmd/xchat-server/main_test.go` 验证 flag 和环境变量结果。

- [ ] **Step 5: 运行目标测试确认通过**

Run:

```sh
env GOCACHE=/tmp/spacechat-go-build-cache go test ./internal/server ./cmd/xchat-server
```

Expected: PASS。

- [ ] **Step 6: 提交**

```sh
git add internal/server/installer_catalog.go internal/server/installer_catalog_test.go cmd/xchat-server/main.go cmd/xchat-server/main_test.go
git commit -m "feat:支持按请求生成客户端安装地址"
```

### Task 3: 仅授权安装配置中的远程 WS

**Files:**
- Modify: `internal/clientconfig/config.go`
- Modify: `internal/clientconfig/config_test.go`
- Modify: `cmd/xchat/main.go`
- Modify: `cmd/xchat/main_test.go`
- Modify: `deploy/client/install.sh`
- Modify: `deploy/client/install.ps1`
- Modify: `deploy/test_client_install.py`

**Interfaces:**
- Produces: `clientconfig.Config.AllowInsecure bool`，JSON 字段 `allow_insecure`，缺省为 `false`。
- Behavior: 只有未传 `--server` 且读取安装配置时使用该字段；命令行地址只接受命令行 `--allow-insecure`。

- [ ] **Step 1: 为配置兼容性和权限边界编写测试**

在 `internal/clientconfig/config_test.go` 覆盖以下 JSON：

```go
valid := []struct {
	raw string
	want bool
}{
	{`{"server":"wss://chat.example/ws"}`, false},
	{`{"server":"ws://chat.example/ws","allow_insecure":true}`, true},
}
invalid := []string{
	`{"server":"ws://chat/ws","allow_insecure":"yes"}`,
	`{"server":"ws://chat/ws","allow_insecure":true,"allow_insecure":false}`,
	`{"server":"ws://chat/ws","extra":true}`,
}
```

在 `cmd/xchat/main_test.go` 增加：配置地址为远程 WS 且 `allow_insecure=true` 时成功；同一配置存在时，显式 `--server ws://other.example/ws` 无 `--allow-insecure` 仍返回配置错误。

在 `deploy/test_client_install.py` 分别执行 WS 与 WSS Linux 安装，断言 JSON 精确为：

```python
{"server": "ws://chat.invalid/ws", "allow_insecure": True}
{"server": "wss://chat.invalid/ws", "allow_insecure": False}
```

Windows PowerShell 安装器不在 Linux 测试中做源码字符串断言；它的配置结果由 Windows 人工安装验收覆盖，Go 配置解析和 Linux 安装器测试负责自动化验证同一 JSON 契约。

- [ ] **Step 2: 运行测试并确认失败**

Run:

```sh
env GOCACHE=/tmp/spacechat-go-build-cache go test ./internal/clientconfig ./cmd/xchat
python3 -m unittest deploy/test_client_install.py
```

Expected: FAIL，配置解码拒绝新字段，安装器只写入 `server`。

- [ ] **Step 3: 扩展严格 JSON 解码器**

将结构和字段分支改为：

```go
type Config struct {
	Server        string `json:"server"`
	AllowInsecure bool   `json:"allow_insecure"`
}

switch name {
case "server":
	if seenServer { return Config{}, errors.New("client config contains duplicate server fields") }
	seenServer = true
	err = decoder.Decode(&config.Server)
case "allow_insecure":
	if seenAllowInsecure { return Config{}, errors.New("client config contains duplicate allow_insecure fields") }
	seenAllowInsecure = true
	err = decoder.Decode(&config.AllowInsecure)
default:
	return Config{}, errors.New("client config contains an unsupported field")
}
```

仍要求 `server` 必须存在；缺少 `allow_insecure` 使用 Go 零值。

- [ ] **Step 4: 在客户端运行入口限制授权来源**

使用独立有效值，避免配置授权泄漏到命令行地址：

```go
address := parsedOptions.server
allowInsecure := parsedOptions.allowInsecure
if !parsedOptions.serverSet {
	config, loadError := clientconfig.Load(layout.Config, defaultServer)
	if loadError != nil {
		fmt.Fprintln(stderr, "spacechat:", loadError)
		return 2
	}
	address = config.Server
	allowInsecure = config.AllowInsecure || parsedOptions.allowInsecure
}
if err = client.ValidateTransport(address, allowInsecure); err != nil {
	fmt.Fprintln(stderr, "spacechat:", err)
	return 2
}
networkOptions := client.Options{AllowInsecure: allowInsecure}
```

- [ ] **Step 5: 让两个安装器写入明确布尔值**

Linux：

```sh
allow_insecure=false
case "$server" in ws://*) allow_insecure=true ;; esac
printf '{"server":"%s","allow_insecure":%s}\n' "$server" "$allow_insecure" > "$config_temporary"
```

Windows：

```powershell
$configJson = @{
    server = $Server
    allow_insecure = ($serverUri.Scheme -eq "ws")
} | ConvertTo-Json -Compress
```

- [ ] **Step 6: 运行目标测试确认通过**

Run:

```sh
env GOCACHE=/tmp/spacechat-go-build-cache go test ./internal/clientconfig ./cmd/xchat
python3 -m unittest deploy/test_client_install.py
```

Expected: PASS。

- [ ] **Step 7: 提交**

```sh
git add internal/clientconfig/config.go internal/clientconfig/config_test.go cmd/xchat/main.go cmd/xchat/main_test.go deploy/client/install.sh deploy/client/install.ps1 deploy/test_client_install.py
git commit -m "feat:记录安装地址的明文连接授权"
```

### Task 4: 安全初始化数据库加密密钥

**Files:**
- Modify: `internal/securestore/key.go`
- Create: `internal/securestore/key_test.go`
- Modify: `cmd/xchat-server/main.go`
- Modify: `cmd/xchat-server/main_test.go`

**Interfaces:**
- Produces: `func EnsureKey(path, databasePath string) error`。
- Produces: `serverConfig.initializeEncryptionKey bool` 和 `-init-encryption-key` flag。
- Behavior: 只在 flag 开启时初始化；传统 systemd 部署继续由安装脚本生成密钥。

- [ ] **Step 1: 编写首次创建、保留和拒绝恢复测试**

新建 `internal/securestore/key_test.go`：

```go
func TestEnsureKeyCreatesAndPreservesRandomKey(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "encryption.key")
	database := filepath.Join(directory, "rooms.db")
	if err := EnsureKey(path, database); err != nil { t.Fatal(err) }
	first, err := ReadKey(path)
	if err != nil || len(first) != 32 { t.Fatalf("key=%x err=%v", first, err) }
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 { t.Fatalf("mode=%o", info.Mode().Perm()) }
	if err = EnsureKey(path, database); err != nil { t.Fatal(err) }
	second, _ := ReadKey(path)
	if !bytes.Equal(first, second) { t.Fatal("existing key was replaced") }
}

func TestEnsureKeyRefusesDatabaseWithoutKey(t *testing.T) {
	directory := t.TempDir()
	database := filepath.Join(directory, "rooms.db")
	if err := os.WriteFile(database, []byte("existing"), 0600); err != nil { t.Fatal(err) }
	if err := EnsureKey(filepath.Join(directory, "missing.key"), database); err == nil {
		t.Fatal("existing database received a new key")
	}
}
```

另测现有无效 key 不被替换、key/database 符号链接被拒绝、创建内容以单个换行结尾且可被 `ReadKey` 读取。

- [ ] **Step 2: 运行测试并确认失败**

Run:

```sh
env GOCACHE=/tmp/spacechat-go-build-cache go test ./internal/securestore ./cmd/xchat-server
```

Expected: FAIL，`EnsureKey` 未定义。

- [ ] **Step 3: 实现独占、可持久化的密钥创建**

`EnsureKey` 的关键顺序必须是：

```go
func EnsureKey(path, databasePath string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 { return errors.New("encryption key path must be a regular file") }
		return nil
	} else if !errors.Is(err, os.ErrNotExist) { return err }
	if _, err := os.Lstat(databasePath); err == nil {
		return errors.New("existing database requires its original encryption key")
	} else if !errors.Is(err, os.ErrNotExist) { return err }
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0750); err != nil { return err }
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("encryption key parent must be a real directory")
	}
	temporary, err := os.CreateTemp(parent, ".encryption-key-")
	if err != nil { return err }
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	closed := false
	defer func() { if !closed { _ = temporary.Close() } }()
	if err = temporary.Chmod(0600); err != nil { return err }
	material := make([]byte, 32)
	if _, err = rand.Read(material); err != nil { return err }
	if _, err = fmt.Fprintln(temporary, base64.StdEncoding.EncodeToString(material)); err != nil { return err }
	if err = temporary.Sync(); err != nil { return err }
	if err = temporary.Close(); err != nil { return err }
	closed = true
	if err = os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, fs.ErrExist) { return nil }
		return err
	}
	directory, err := os.Open(parent)
	if err != nil { return err }
	defer directory.Close()
	return directory.Sync()
}
```

导入 `crypto/rand`、`io/fs`、`fmt`、`os`、`path/filepath`。任何失败都删除临时文件；若 `os.Link` 返回目标已存在，则不覆盖，交给随后 `ReadKey` 严格验证。

- [ ] **Step 4: 接入服务端显式 flag**

解析：

```go
set.BoolVar(&config.initializeEncryptionKey, "init-encryption-key", false, "Create a database encryption key only when both key and database are absent")
```

在 `securestore.ReadKey` 前执行：

```go
if config.initializeEncryptionKey {
	if err := securestore.EnsureKey(config.encryptionKeyFile, config.databasePath); err != nil {
		slog.Error("initialize encryption key", "error", err)
		os.Exit(1)
	}
}
```

扩展解析测试，确认默认关闭、flag 开启，并确保 `-validate-updates` 路径仍在触碰数据库密钥前返回。

- [ ] **Step 5: 运行目标测试确认通过**

Run:

```sh
env GOCACHE=/tmp/spacechat-go-build-cache go test ./internal/securestore ./cmd/xchat-server
```

Expected: PASS。

- [ ] **Step 6: 提交**

```sh
git add internal/securestore/key.go internal/securestore/key_test.go cmd/xchat-server/main.go cmd/xchat-server/main_test.go
git commit -m "feat:支持容器首次启动生成数据库密钥"
```

### Task 5: 每日上海午夜清理调度

**Files:**
- Modify: `deploy/clear_history.py`
- Modify: `deploy/test_clear_history.py`

**Interfaces:**
- Produces: `seconds_until_next_midnight(now)`，输入 aware `datetime`，返回正秒数。
- Produces: `run_daily(socket_path, now, sleep, clear, stderr)`，依赖可注入以便测试。
- Produces: CLI flag `--schedule-daily`，与交互/`--yes` 一次性模式互斥。

- [ ] **Step 1: 编写时区、错过不补跑和失败不重试测试**

在 `deploy/test_clear_history.py` 增加：

```python
def test_next_midnight_uses_fixed_shanghai_time(self):
    now = datetime.datetime(2026, 9, 22, 15, 30, tzinfo=datetime.timezone.utc)
    self.assertEqual(1800, clear_history.seconds_until_next_midnight(now))
    midnight = datetime.datetime(2026, 9, 22, 16, 0, tzinfo=datetime.timezone.utc)
    self.assertEqual(24 * 60 * 60, clear_history.seconds_until_next_midnight(midnight))

def test_schedule_failure_waits_for_the_next_midnight(self):
    sleeps = []
    def stop_after_second_sleep(seconds):
        sleeps.append(seconds)
        if len(sleeps) == 2:
            raise StopIteration
    with self.assertRaises(StopIteration):
        clear_history.run_daily(
            "/run/admin.sock",
            now=lambda zone: datetime.datetime(2026, 9, 22, 15, 30, tzinfo=zone),
            sleep=stop_after_second_sleep,
            clear=mock.Mock(side_effect=OSError("offline")),
            stderr=io.StringIO(),
        )
    self.assertEqual(2, len(sleeps))
```

另测 `main(["--schedule-daily", "--yes"])` 返回参数错误，以及一次性模式行为不变。

- [ ] **Step 2: 运行测试并确认失败**

Run:

```sh
python3 -m unittest deploy/test_clear_history.py
```

Expected: FAIL，调度函数和 flag 不存在。

- [ ] **Step 3: 实现可测试的调度循环**

核心实现：

```python
SHANGHAI = datetime.timezone(datetime.timedelta(hours=8), name="Asia/Shanghai")

def seconds_until_next_midnight(now):
    local = now.astimezone(SHANGHAI)
    next_date = local.date() + datetime.timedelta(days=1)
    target = datetime.datetime.combine(next_date, datetime.time.min, SHANGHAI)
    return max(1, int((target - local).total_seconds()))

def run_daily(socket_path, now=datetime.datetime.now, sleep=time.sleep,
              clear=clear_history, stderr=sys.stderr):
    while True:
        sleep(seconds_until_next_midnight(now(datetime.timezone.utc)))
        try:
            deleted = clear(socket_path)
            print("清理成功，已删除 {} 条聊天记录。".format(deleted))
        except (OSError, http.client.HTTPException, ValueError, RuntimeError) as error:
            print("定时清理失败：{}；将在下一个北京时间午夜重试。".format(error), file=stderr)
```

为避免 Python 默认参数过早绑定输出流，实际实现可把 `stderr=None` 并在函数内解析为 `sys.stderr`；测试接口名称保持不变。

- [ ] **Step 4: 接入 CLI 并验证**

使用 argparse 互斥组处理 `--yes` 与 `--schedule-daily`。调度模式不读取 stdin，收到 SIGTERM 时由容器正常终止；一次性模式保持现有确认提示和退出码。

```python
mode = parser.add_mutually_exclusive_group()
mode.add_argument("--yes", action="store_true", help="跳过交互确认，立即执行清空")
mode.add_argument("--schedule-daily", action="store_true", help="每天北京时间 00:00 执行清空")
arguments = parser.parse_args(argv)
if arguments.schedule_daily:
    run_daily(arguments.socket)
    return 0
```

Run:

```sh
python3 -m unittest deploy/test_clear_history.py deploy/test_rooms_deploy.py
```

Expected: PASS。

- [ ] **Step 5: 提交**

```sh
git add deploy/clear_history.py deploy/test_clear_history.py
git commit -m "feat:增加容器每日历史清理调度"
```

### Task 6: 可替换密钥的客户端签名制品构建器

**Files:**
- Create: `deploy/docker/build-artifacts.sh`
- Create: `deploy/docker/default-update-signing.seed`
- Create: `deploy/docker/empty-client-ca.pem`
- Create: `deploy/test_docker_artifacts.py`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: `/run/secrets/update-signing.key`、`/run/config/client-ca.pem`、`/opt/windows-terminal`、仓库源码与 `spacechat-release`。
- Consumes env: `SPACECHAT_VERSION`、`SPACECHAT_MINIMUM_VERSION`、`SPACECHAT_OUTPUT_DIR`；测试可通过 `SPACECHAT_SIGNING_KEY_PATH`、`SPACECHAT_CLIENT_CA_PATH`、`SPACECHAT_TERMINAL_DIR`、`SPACECHAT_RELEASE_BIN`、`SPACECHAT_SERVER_BIN` 覆盖外部输入路径。
- Produces: `${SPACECHAT_OUTPUT_DIR}/release/{updates,installers,update-public.key}`，只有完整验证后才替换。
- Produces: Windows/Linux amd64 稳定启动器、版本化客户端与首装 ZIP。

- [ ] **Step 1: 编写真实执行的构建器测试**

新建 `deploy/test_docker_artifacts.py`：

```python
import base64
import json
import os
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class DockerArtifactBuilderTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temporary.name)
        self.release_tool = self.root / "spacechat-release"
        self.server = self.root / "xchat-server"
        subprocess.run(["go", "build", "-o", self.release_tool, "./cmd/spacechat-release"], cwd=ROOT, check=True)
        subprocess.run(["go", "build", "-o", self.server, "./cmd/xchat-server"], cwd=ROOT, check=True)
        self.terminal = self.root / "terminal"
        (self.terminal / "settings").mkdir(parents=True)
        (self.terminal / "WindowsTerminal.exe").write_bytes(b"terminal")
        (self.terminal / ".portable").write_text("", encoding="utf-8")
        (self.terminal / "settings/settings.json").write_text("{}\n", encoding="utf-8")

    def tearDown(self):
        self.temporary.cleanup()

    def environment(self):
        environment = os.environ.copy()
        environment.update({
            "SPACECHAT_OUTPUT_DIR": str(self.root / "artifacts"),
            "SPACECHAT_SIGNING_KEY_PATH": str(ROOT / "deploy/docker/default-update-signing.seed"),
            "SPACECHAT_CLIENT_CA_PATH": str(ROOT / "deploy/docker/empty-client-ca.pem"),
            "SPACECHAT_TERMINAL_DIR": str(self.terminal),
            "SPACECHAT_RELEASE_BIN": str(self.release_tool),
            "SPACECHAT_SERVER_BIN": str(self.server),
            "GOCACHE": "/tmp/spacechat-go-build-cache",
        })
        return environment

    def test_default_seed_is_accepted_by_the_real_release_tool(self):
        result = subprocess.run(
            [self.release_tool, "public-key", "-private-key", ROOT / "deploy/docker/default-update-signing.seed"],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=True,
        )
        self.assertEqual(32, len(base64.b64decode(result.stdout.strip(), validate=True)))

    def test_builder_generates_and_atomically_replaces_a_verified_release(self):
        command = ["sh", str(ROOT / "deploy/docker/build-artifacts.sh")]
        first = subprocess.run(command, cwd=ROOT, env=self.environment(), stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=True)
        release = self.root / "artifacts/release"
        self.assertEqual(2, json.loads((release / "installers/installers.json").read_text())["schema"])
        self.assertTrue((release / "updates/manifest.sig").is_file())
        self.assertEqual(32, len(base64.b64decode((release / "update-public.key").read_text().strip(), validate=True)))
        marker = release / "obsolete"
        marker.write_text("old", encoding="utf-8")
        second = subprocess.run(command, cwd=ROOT, env=self.environment(), stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=True)
        self.assertFalse(marker.exists())
        seed = (ROOT / "deploy/docker/default-update-signing.seed").read_text().strip()
        self.assertNotIn(seed, first.stdout + first.stderr + second.stdout + second.stderr)

if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: 运行测试并确认失败**

Run:

```sh
python3 -m unittest deploy/test_docker_artifacts.py
```

Expected: FAIL，seed 和构建脚本不存在。

- [ ] **Step 3: 生成并标记公开默认 seed**

Run:

```sh
openssl rand -base64 32
```

将输出通过 `apply_patch` 写入 `deploy/docker/default-update-signing.seed`，文件只含一行 base64 和换行。不要把它命名为正式发布密钥。创建零字节 `deploy/docker/empty-client-ca.pem`，供 Compose 默认只读挂载。

`.gitignore` 增加：

```gitignore
/.env
/deploy/docker/tls/*
!/deploy/docker/tls/README.md
```

- [ ] **Step 4: 实现输入校验和编译元数据**

`build-artifacts.sh` 开头必须包含：

```sh
#!/bin/sh
set -eu
umask 077

version=${SPACECHAT_VERSION:-0.4.0}
minimum=${SPACECHAT_MINIMUM_VERSION:-0.0.0}
output=${SPACECHAT_OUTPUT_DIR:-/artifacts}
signing_key=${SPACECHAT_SIGNING_KEY_PATH:-/run/secrets/update-signing.key}
client_ca=${SPACECHAT_CLIENT_CA_PATH:-/run/config/client-ca.pem}
terminal_dir=${SPACECHAT_TERMINAL_DIR:-/opt/windows-terminal}
release_bin=${SPACECHAT_RELEASE_BIN:-/usr/local/bin/spacechat-release}
server_bin=${SPACECHAT_SERVER_BIN:-/usr/local/bin/xchat-server}
private_key=$(mktemp)
staging=$(mktemp -d "$output/.release.XXXXXX")
trap 'rm -f "$private_key"; rm -rf "$staging"' EXIT HUP INT TERM
install -m 0600 "$signing_key" "$private_key"
public_key=$($release_bin public-key -private-key "$private_key")
```

校验 `version`/`minimum` 使用与发布工具相同的三段式格式；`client-ca.pem` 非空时必须包含证书且不得包含私钥，然后用 `base64 | tr -d '\n'` 生成链接值。

- [ ] **Step 5: 交叉编译、打包并签名**

使用以下确定性输出布局和链接参数：

```sh
client_ldflags="-s -w -X main.defaultServer=ws://127.0.0.1:18081/ws -X main.version=$version -X main.updatePublicKey=$public_key"
if [ -s "$client_ca" ]; then
    encoded_ca=$(base64 "$client_ca" | tr -d '\n')
    client_ldflags="$client_ldflags -X main.defaultTLSCA=$encoded_ca"
fi

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o "$staging/spacechat.exe" ./cmd/spacechat
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$client_ldflags" -o "$staging/spacechat-client.exe" ./cmd/xchat
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o "$staging/spacechat" ./cmd/spacechat
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$client_ldflags" -o "$staging/spacechat-client" ./cmd/xchat
```

创建 Windows/Linux 包目录，复制两个安装脚本和 README；Windows 包复制 `$terminal_dir`，再复制 `scripts/windows-terminal/.portable` 和 `settings.json` 到与现有 PowerShell 发布包相同的位置。用 `zip -qr` 生成两个 ZIP，然后执行：

```sh
install -d -m 0700 "$staging/release"
$release_bin manifest \
  -version "$version" -minimum "$minimum" -private-key "$private_key" \
  -windows "$staging/spacechat-client.exe" -linux "$staging/spacechat-client" \
  -out "$staging/release/updates"
$release_bin installers \
  -version "$version" -server-mode request -private-key "$private_key" \
  -windows-package "$staging/spacechat-windows-amd64-$version.zip" \
  -linux-package "$staging/spacechat-linux-amd64-$version.zip" \
  -out "$staging/release/installers"
printf '%s\n' "$public_key" > "$staging/release/update-public.key"
```

使用 `$server_bin -validate-updates` 对 staging 中 updates/installers/key 做最终验证。

验证后把发布目录设为只读服务可读取、不可写的权限；私钥不在该目录中：

```sh
find "$staging/release" -type d -exec chmod 0755 {} \;
find "$staging/release" -type f -exec chmod 0644 {} \;
```

- [ ] **Step 6: 原子发布完整 release 目录**

在同一命名卷内执行单目录切换：删除旧 `release.previous`，把现有 `release` 改名为 `release.previous`，把 staging 中的 `release` 改名为当前目录；若第二次改名失败则恢复 previous。成功后删除 previous，清除 trap 中 staging 路径。私钥临时文件无论成功失败都删除。

```sh
current=$output/release
previous=$output/release.previous
rm -rf "$previous"
had_current=0
if [ -d "$current" ]; then
    mv "$current" "$previous"
    had_current=1
fi
if ! mv "$staging/release" "$current"; then
    if [ "$had_current" -eq 1 ]; then mv "$previous" "$current"; fi
    exit 1
fi
rm -rf "$previous"
```

- [ ] **Step 7: 运行契约测试**

Run:

```sh
python3 -m unittest deploy/test_docker_artifacts.py
sh -n deploy/docker/build-artifacts.sh
```

Expected: PASS。

- [ ] **Step 8: 提交**

```sh
git add .gitignore deploy/docker/build-artifacts.sh deploy/docker/default-update-signing.seed deploy/docker/empty-client-ca.pem deploy/test_docker_artifacts.py
git commit -m "feat:增加容器客户端签名制品构建器"
```

### Task 7: 多阶段运行镜像与 TLS 入口

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`
- Create: `deploy/docker/server-entrypoint.sh`
- Create: `deploy/docker/healthcheck.sh`
- Create: `deploy/docker/tls/README.md`
- Create: `deploy/docker/test-images.sh`

**Interfaces:**
- Produces Docker targets: `artifacts`、`server`、`cleanup`。
- `server-entrypoint.sh` consumes: `SPACECHAT_ALLOW_CIDR`、`SPACECHAT_PUBLIC_URL`、`/tls/tls.crt`、`/tls/tls.key`。
- `server` expects: `/artifacts/release` read-only、`/data` writable、`/run/spacechat` writable。
- `cleanup` entrypoint: `python3 /app/clear_history.py`。

- [ ] **Step 1: 编写真实镜像行为测试**

新建可执行的 `deploy/docker/test-images.sh`。脚本构建 `server`、`cleanup`、`artifacts` 三个 target，然后通过实际容器行为验证：

- server 镜像不含 Go 工具链和签名私钥，且 `su-exec spacechat id -u` 输出 `10001`；
- 只挂载 `tls.crt` 时，真实 server entrypoint 非零退出，标准错误包含 `TLS certificate and private key must be mounted together`；
- `docker run --rm spacechat-cleanup:test --help` 展示 `--schedule-daily`；
- 在仓库根目录临时创建 `.docker-secret.key` 后重新构建 artifacts target，并覆盖 entrypoint 检查镜像 `/src` 中不存在该文件，以实际 build context 行为验证 `.dockerignore`；脚本用 trap 无条件删除临时文件和目录。

脚本允许通过 `SPACECHAT_SERVER_IMAGE`、`SPACECHAT_CLEANUP_IMAGE`、`SPACECHAT_ARTIFACTS_IMAGE` 覆盖测试镜像名，默认分别使用 `spacechat-server:test`、`spacechat-cleanup:test`、`spacechat-artifacts:test`。

- [ ] **Step 2: 运行测试并确认失败**

Run:

```sh
sh deploy/docker/test-images.sh
```

Expected: FAIL，Dockerfile 与入口文件不存在。

- [ ] **Step 3: 实现服务端入口和健康检查**

`server-entrypoint.sh` 使用 `set --` 安全追加 POSIX shell 参数：

```sh
#!/bin/sh
set -eu
cert=/tls/tls.crt
key=/tls/tls.key
if [ -f "$cert" ] && [ -f "$key" ]; then
    install -d -m 0700 -o spacechat -g spacechat /run/spacechat-tls
    install -m 0644 -o spacechat -g spacechat "$cert" /run/spacechat-tls/tls.crt
    install -m 0600 -o spacechat -g spacechat "$key" /run/spacechat-tls/tls.key
    set -- -tls-cert /run/spacechat-tls/tls.crt -tls-key /run/spacechat-tls/tls.key
elif [ -e "$cert" ] || [ -e "$key" ]; then
    printf '%s\n' 'TLS certificate and private key must be mounted together.' >&2
    exit 1
else
    set -- -allow-insecure
fi
if [ -n "${SPACECHAT_PUBLIC_URL:-}" ]; then
    set -- "$@" -public-url "$SPACECHAT_PUBLIC_URL"
fi
chown spacechat:spacechat /data /run/spacechat
exec su-exec spacechat /usr/local/bin/xchat-server \
  -listen 0.0.0.0:18081 -db /data/rooms.db \
  -allow-cidr "${SPACECHAT_ALLOW_CIDR:-127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16}" \
  -encryption-key-file /data/encryption.key -init-encryption-key \
  -admin-socket /run/spacechat/admin.sock \
  -update-dir /artifacts/release/updates \
  -installer-dir /artifacts/release/installers \
  -update-public-key-file /artifacts/release/update-public.key "$@"
```

`healthcheck.sh` 检测证书对是否存在，分别用 BusyBox `wget` 请求 HTTP 或带 `--no-check-certificate` 的 HTTPS 本机地址；只用于容器内存活检查：

```sh
#!/bin/sh
set -eu
if [ -f /tls/tls.crt ] && [ -f /tls/tls.key ]; then
    exec wget -q --no-check-certificate -O /dev/null https://127.0.0.1:18081/healthz
fi
exec wget -q -O /dev/null http://127.0.0.1:18081/healthz
```

- [ ] **Step 4: 编写多阶段 Dockerfile**

固定基础镜像与主要步骤：

```dockerfile
FROM golang:1.26.0-bookworm AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/xchat-server ./cmd/xchat-server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/spacechat-release ./cmd/spacechat-release

FROM golang:1.26.0-bookworm AS artifacts
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl unzip zip \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY --from=go-build /out/xchat-server /out/spacechat-release /usr/local/bin/
COPY --from=go-build /src /src
RUN curl -fL -o /tmp/terminal.zip https://github.com/microsoft/terminal/releases/download/v1.24.11911.0/Microsoft.WindowsTerminal_1.24.11911.0_x64.zip \
 && echo "7691efeb71c8dd0b95536c84e366fa4cf809a42c534912f9cefa1056534383bd  /tmp/terminal.zip" | sha256sum -c - \
 && unzip -q /tmp/terminal.zip -d /tmp/terminal \
 && mv /tmp/terminal/terminal-1.24.11911.0 /opt/windows-terminal \
 && rm -rf /tmp/terminal.zip /tmp/terminal \
 && chmod 0755 /src/deploy/docker/build-artifacts.sh
ENTRYPOINT ["/src/deploy/docker/build-artifacts.sh"]
```

`server` 使用 `alpine:3.22`，安装 `su-exec`，创建 UID/GID 10001 的 `spacechat`，复制服务端、入口和健康脚本。入口以 root 进行一次性的卷权限修正和 TLS 私钥复制，随后用 `su-exec spacechat` 替换自身；长期运行的服务进程不是 root。TLS 私钥副本只存在于容器自己的 `/run/spacechat-tls`，不进入共享管理 socket 卷。`cleanup` 使用 `python:3.13-alpine3.22`，创建相同 UID/GID，复制 `clear_history.py`，并以 `USER spacechat` 运行。

- [ ] **Step 5: 添加 TLS 目录说明和 Docker 上下文排除**

`deploy/docker/tls/README.md` 明确只接受同目录 `tls.crt` 与 `tls.key`，证书必须匹配客户端使用的域名/IP；实际文件被 Git 忽略。`.dockerignore` 至少包含：

```dockerignore
.git
.worktrees
dist
.env
*.db
*.db-shm
*.db-wal
*.key
deploy/docker/tls/tls.crt
deploy/docker/tls/tls.key
```

- [ ] **Step 6: 运行脚本检查和真实镜像测试**

Run:

```sh
sh -n deploy/docker/server-entrypoint.sh deploy/docker/healthcheck.sh
sh -n deploy/docker/test-images.sh
sh deploy/docker/test-images.sh
```

Expected: 全部 PASS，三个 target 构建成功且运行行为符合约束。

- [ ] **Step 7: 验证运行镜像隔离**

Run:

```sh
docker run --rm --entrypoint sh spacechat-server:test -c 'test ! -e /usr/local/go && test ! -e /run/secrets/update-signing.key && su-exec spacechat id -u | grep -x 10001'
```

Expected: exit 0，输出 `10001`。Task 8 的运行中验收还必须通过 `docker compose top server -eo uid,pid,comm` 确认服务进程 UID 为 10001。

- [ ] **Step 8: 提交**

```sh
git add Dockerfile .dockerignore deploy/docker/server-entrypoint.sh deploy/docker/healthcheck.sh deploy/docker/tls/README.md deploy/docker/test-images.sh
git commit -m "feat:增加SpaceChat容器运行镜像"
```

### Task 8: Compose 编排与端到端冒烟测试

**Files:**
- Create: `compose.yaml`
- Create: `deploy/test_docker_compose.py`
- Create: `deploy/docker/smoke-test.sh`

**Interfaces:**
- Compose services: `artifacts`、`server`、`cleanup`。
- Named volumes: `release-assets`、`data`、`admin-runtime`。
- Host inputs: `${SPACECHAT_SIGNING_KEY_FILE}`、`${SPACECHAT_CLIENT_CA_FILE}`、`${SPACECHAT_TLS_DIR}`。
- Service dependency: `server` waits for `artifacts` successful completion; `cleanup` waits for `server` health.

- [ ] **Step 1: 编写 Compose 结构测试**

新建 `deploy/test_docker_compose.py`，调用 `docker compose config --format json` 并断言：

```python
configuration = json.loads(subprocess.check_output(
    ["docker", "compose", "-f", str(ROOT / "compose.yaml"), "config", "--format", "json"],
    cwd=ROOT,
    text=True,
))
self.assertEqual({"artifacts", "server", "cleanup"}, set(configuration["services"]))
self.assertEqual("service_completed_successfully", configuration["services"]["server"]["depends_on"]["artifacts"]["condition"])
self.assertEqual("service_healthy", configuration["services"]["cleanup"]["depends_on"]["server"]["condition"])
self.assertEqual({"release-assets", "data", "admin-runtime"}, set(configuration["volumes"]))
```

另断言 signing key mount 只存在于 `artifacts`，制品对 `server` 为只读，TLS 目录只存在于 `server`，cleanup 只挂载 admin runtime，server 端口 target 为 18081，server/cleanup 使用 `unless-stopped`。

- [ ] **Step 2: 运行结构测试并确认失败**

Run:

```sh
python3 -m unittest deploy/test_docker_compose.py
```

Expected: FAIL，`compose.yaml` 不存在。

- [ ] **Step 3: 编写 Compose 文件**

核心定义必须等价于：

```yaml
services:
  artifacts:
    build:
      context: .
      target: artifacts
    environment:
      SPACECHAT_VERSION: ${SPACECHAT_VERSION:-0.4.0}
      SPACECHAT_MINIMUM_VERSION: ${SPACECHAT_MINIMUM_VERSION:-0.0.0}
      SPACECHAT_OUTPUT_DIR: /artifacts
    volumes:
      - type: bind
        source: ${SPACECHAT_SIGNING_KEY_FILE:-./deploy/docker/default-update-signing.seed}
        target: /run/secrets/update-signing.key
        read_only: true
        bind:
          create_host_path: false
      - type: bind
        source: ${SPACECHAT_CLIENT_CA_FILE:-./deploy/docker/empty-client-ca.pem}
        target: /run/config/client-ca.pem
        read_only: true
        bind:
          create_host_path: false
      - release-assets:/artifacts
    restart: "no"

  server:
    build:
      context: .
      target: server
    depends_on:
      artifacts:
        condition: service_completed_successfully
    environment:
      SPACECHAT_ALLOW_CIDR: ${SPACECHAT_ALLOW_CIDR:-127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16}
      SPACECHAT_PUBLIC_URL: ${SPACECHAT_PUBLIC_URL:-}
    ports:
      - "${SPACECHAT_PORT:-18081}:18081"
    volumes:
      - release-assets:/artifacts:ro
      - data:/data
      - admin-runtime:/run/spacechat
      - type: bind
        source: ${SPACECHAT_TLS_DIR:-./deploy/docker/tls}
        target: /tls
        read_only: true
        bind:
          create_host_path: false
    healthcheck:
      test: ["CMD", "/usr/local/bin/healthcheck.sh"]
      interval: 5s
      timeout: 3s
      retries: 12
    restart: unless-stopped
    stop_grace_period: 15s

  cleanup:
    build:
      context: .
      target: cleanup
    depends_on:
      server:
        condition: service_healthy
    command: ["--schedule-daily", "--socket", "/run/spacechat/admin.sock"]
    volumes:
      - admin-runtime:/run/spacechat
    restart: unless-stopped

volumes:
  release-assets:
  data:
  admin-runtime:
```

- [ ] **Step 4: 编写隔离的 WS 冒烟测试**

`deploy/docker/smoke-test.sh` 使用唯一项目名并用 trap 只删除该项目：

```sh
#!/bin/sh
set -eu
project="spacechat-smoke-$$"
cleanup() { docker compose -p "$project" down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT HUP INT TERM
SPACECHAT_PORT=0 docker compose -p "$project" up -d --build
port=$(docker compose -p "$project" port server 18081)
port=${port##*:}
```

循环最多 60 次请求 `http://127.0.0.1:$port/healthz`；随后验证 `/install/linux` 包含 `ws://127.0.0.1:$port/ws`、`/install/windows` 可访问、`/updates/v1/manifest` 和 `/install/v1/manifest` 可访问。用 `docker compose -p "$project" top server -eo uid,pid,comm` 断言 `xchat-server` 的 UID 为 10001。记录容器内 `/data/encryption.key` 的 SHA-256，重启 server 后再次比较，必须一致。最后执行：

```sh
docker compose -p "$project" run --rm cleanup --yes --socket /run/spacechat/admin.sock
```

确认返回成功；trap 的 `down -v` 只能作用于脚本生成的唯一项目。

- [ ] **Step 5: 增加 TLS 入口验收**

冒烟脚本的 `--tls` 模式使用宿主机 `openssl req -x509 -newkey rsa:2048 -nodes -subj /CN=localhost -addext subjectAltName=DNS:localhost,IP:127.0.0.1` 在临时目录生成 `tls.crt`/`tls.key`，先选择未占用端口并设置 `SPACECHAT_TLS_DIR` 与 `SPACECHAT_PUBLIC_URL=wss://localhost:$port/ws`，再启动另一个唯一 Compose 项目，并用 `curl --cacert` 验证 HTTPS health 和安装脚本中的 WSS 地址。只提供证书不提供 key 的子测试必须观察到 server 非零/重启状态，日志包含成对错误。

- [ ] **Step 6: 运行结构和端到端测试**

Run:

```sh
python3 -m unittest deploy/test_docker_compose.py
sh deploy/docker/smoke-test.sh
sh deploy/docker/smoke-test.sh --tls
```

Expected: PASS；测试结束后不存在以 `spacechat-smoke-` 开头的容器或卷。

- [ ] **Step 7: 提交**

```sh
git add compose.yaml deploy/test_docker_compose.py deploy/docker/smoke-test.sh
git commit -m "feat:增加Docker Compose一键部署编排"
```

### Task 9: 部署文档、回归与最终验收

**Files:**
- Create: `.env.example`
- Modify: `README.md`

**Interfaces:**
- Documents all Compose variables and operational commands without requiring source knowledge。
- Does not add runtime behavior beyond Tasks 1–8。

- [ ] **Step 1: 写一键部署与客户端安装说明**

在 README 的“服务端部署”前新增 Docker Compose 首选路径：

```sh
git clone https://github.com/spacebluez/spacechat.git
cd spacechat
docker compose up -d --build
docker compose ps
docker compose logs -f server
```

明确默认服务为可信内网 WS，并给出 Linux/Windows 首装 URL。说明访问首装 URL 的 Host 会成为客户端连接地址；反向代理必须配置 `SPACECHAT_PUBLIC_URL`。

- [ ] **Step 2: 写密钥、TLS、升级、备份与清理说明**

文档必须明确：

- 默认更新签名 seed 公开，任何人可签名，不代表发布者身份认证；正式环境建议通过 `SPACECHAT_SIGNING_KEY_FILE` 替换，但程序不强制。
- 更换更新私钥后执行 `docker compose down` 再 `docker compose up -d --build`，并重新安装与新公钥匹配的客户端。
- 数据库密钥自动随机生成，数据库与密钥在同一数据卷中但仍需成套备份；所有部署共享数据库密钥没有收益。
- `docker compose down` 保留卷；`docker compose down -v` 永久删除数据库和密钥，使用醒目的“不可恢复”警告。
- `deploy/docker/tls/tls.crt` 与 `tls.key` 成对挂载后启用 WSS；内部 CA 通过 `SPACECHAT_CLIENT_CA_FILE` 编入客户端。
- 公网部署必须在首次启动前启用 WSS、设置 `SPACECHAT_PUBLIC_URL`、限制 `SPACECHAT_ALLOW_CIDR` 并替换默认更新密钥。
- `cleanup` 每天 `Asia/Shanghai` 00:00 清理，手工命令为 `docker compose run --rm cleanup --yes --socket /run/spacechat/admin.sock`。

`.env.example` 使用可直接复制但安全的示例：

```dotenv
SPACECHAT_PORT=18081
SPACECHAT_VERSION=0.4.0
SPACECHAT_MINIMUM_VERSION=0.0.0
SPACECHAT_ALLOW_CIDR=127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16
SPACECHAT_PUBLIC_URL=
SPACECHAT_SIGNING_KEY_FILE=./deploy/docker/default-update-signing.seed
SPACECHAT_CLIENT_CA_FILE=./deploy/docker/empty-client-ca.pem
SPACECHAT_TLS_DIR=./deploy/docker/tls
```

- [ ] **Step 3: 人工核对运维文档契约**

逐项核对 README 和 `.env.example` 是否覆盖：一键启动、Linux/Windows 安装 URL、默认签名密钥信任边界、自有密钥轮换、数据库密钥与备份、WS/WSS 切换、公网部署要求、清理时区与命令、保留卷和永久删卷的区别。自然语言文案不使用源码字符串断言；运行行为由 Tasks 1–8 的测试覆盖。

- [ ] **Step 4: 运行格式化与完整语言测试**

Run:

```sh
gofmt -w internal/update/installer_manifest.go internal/update/installer_manifest_test.go cmd/spacechat-release/main.go cmd/spacechat-release/main_test.go internal/server/installer_catalog.go internal/server/installer_catalog_test.go cmd/xchat-server/main.go cmd/xchat-server/main_test.go internal/clientconfig/config.go internal/clientconfig/config_test.go cmd/xchat/main.go cmd/xchat/main_test.go internal/securestore/key.go internal/securestore/key_test.go
env GOCACHE=/tmp/spacechat-go-build-cache go test ./...
python3 -m unittest discover -s deploy -p 'test_*.py'
git diff --check
```

Expected: 所有 Go/Python 测试 PASS，`git diff --check` 无输出。若沙箱禁止测试 socket，使用同一命令在允许本地 TCP/Unix socket 的环境重跑，不得把权限失败当作产品回归。

- [ ] **Step 5: 运行完整 Docker 验收**

Run:

```sh
docker compose config --quiet
docker build --target artifacts -t spacechat-artifacts:test .
docker build --target server -t spacechat-server:test .
docker build --target cleanup -t spacechat-cleanup:test .
sh deploy/docker/test-images.sh
sh deploy/docker/smoke-test.sh
sh deploy/docker/smoke-test.sh --tls
```

Expected: 镜像构建和 WS/WSS 冒烟测试全部 PASS；测试项目被清理，开发用命名卷和现有部署卷未被触碰。

- [ ] **Step 6: 检查密钥泄漏与提交范围**

Run:

```sh
git status --short
git diff --name-only main...HEAD
git grep -n 'BEGIN.*PRIVATE KEY' -- ':!docs/superpowers' || true
git grep -n 'default-update-signing.seed'
```

Expected: 只有计划内文件；没有 TLS 私钥或用户自有签名私钥；默认公开 seed 只出现在明确的构建与文档路径。

- [ ] **Step 7: 提交文档**

```sh
git add README.md .env.example
git commit -m "docs:补充Docker Compose部署与安全说明"
```

- [ ] **Step 8: 最终分支验证**

Run:

```sh
git status --short --branch
git log --reverse --format='%h %s' main..HEAD
```

Expected: 工作树干净；新增提交标题全部符合仓库 hook 允许的格式，且没有直接修改 `main`。
