# 客户端在线更新验收记录（2026-09-21）

## 环境

- 最终复验时间：2026-09-21 17:04:15 HKT（UTC+08:00）
- 工作分支：`codex/client-online-update`
- Go：`go1.27.1 linux/amd64`
- Python：`3.7.9`
- 支持的发布目标：Windows amd64、Linux amd64

本次验证没有生成或使用正式发布私钥，没有写入仓库内的 `dist` 目录。交叉编译产物只写入 `/tmp/spacechat-verify-final.VeyMq4`。

## 端到端更新验证

```text
env GOCACHE=/tmp/spacechat-go-cache go test ./internal/update -run TestManagedClientUpdateEndToEnd -count=1 -v
```

退出码：`0`。1 个顶层测试及 3 个子测试通过，覆盖 4 条业务路径：

- `0.3.2 -> 0.4.0` 可选更新选择 Skip，`current` 保持 `0.3.2`。
- 同一可选更新选择 Update，通过真实签名验证、HTTP 下载、SHA-256、文件权限、自检执行及原子指针切换后，`current` 为 `0.4.0`。
- `0.3.1 -> 0.4.0` 强制更新收到与清单摘要不符的制品，目标文件被清理，`current` 保持 `0.3.1`，流程只能退出而不能继续聊天。
- 最低版本为 `0.3.2` 时，`0.3.1` 的 WebSocket 加入请求收到的第一帧是 `upgrade_required`，没有先收到历史。

## 格式化、race 与全仓 Go 验证

```text
gofmt -w cmd internal
env GOCACHE=/tmp/spacechat-go-cache go test -race ./internal/update ./internal/server ./internal/client ./internal/tui ./cmd/xchat -count=1
```

退出码：`0`。5 个关键包全部通过，未报告数据竞争。

```text
env GOCACHE=/tmp/spacechat-go-cache go test ./... -count=1
env GOCACHE=/tmp/spacechat-go-cache go vet ./...
```

两个命令最终退出码均为 `0`。全仓 17 个含测试的 Go 包通过，另有 `internal/terminal` 无当前平台测试文件。通过 `go test ./... -list '^Test'` 统计到 119 个 Go 顶层测试；该数字不包含动态子测试。

复审修复后的首次全仓运行中，`TestWrongKeyStopsRetrying` 曾单次达到 10 秒超时；该测试随后独立连续运行 10 次均通过，之后两次全仓重跑也全部通过。最终一次普通文件沙箱运行因环境禁止监听本机临时 TCP/Unix socket 而失败；使用完全相同的测试命令、仅允许本机临时 socket 后重跑，全仓通过。这两类非功能性中间结果均未被记作通过。

## 部署与安装脚本验证

```text
python3 -m unittest discover -s deploy -p 'test_*.py' -v
sh -n deploy/client/install.sh
sh -n deploy/rooms/install.sh
```

退出码：`0`。15 个 Python 测试通过，两个 shell 安装器均通过语法检查。覆盖内容包括：

- Linux 首次安装器在临时 HOME/XDG 目录中的真实安装、自检、严格 JSON 配置、版本指针、执行权限与 PATH 配置。
- Windows 用户级目录和用户级 PATH 的静态约束。
- 构建脚本必须提供签名密钥并生成双平台启动器、客户端和签名清单。
- 服务端部署在切换前校验 `updates.new`，拒绝符号链接，并对更新目录、公钥、服务端、清理脚本及两个 cleanup 单元做整组失败回滚。
- 既有 Unix 管理 socket 清理流程。

普通文件沙箱会禁止既有 Unix socket 测试创建本地 socket；在允许本机临时 socket 的环境中，以完全相同的测试命令运行并全部通过。

## 复审回归覆盖

独立代码复审发现的问题均已加入回归验证：更新后的客户端保持终端前台并将非零退出视为启动失败；制品下载使用独立的 30 分钟超时；安装中断可恢复已验证目标或替换损坏目标；服务端已判定不兼容后只能 Retry/Exit；并发更新完成会校验并复用且不会误回滚；首次安装会核对客户端内嵌版本；服务端发布失败会恢复全部关联资产及 cleanup timer 的运行态。

## 交叉编译

以下命令均退出 `0`：

```text
env GOCACHE=/tmp/spacechat-go-cache GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/spacechat-verify-final.VeyMq4/spacechat-windows-amd64.exe ./cmd/spacechat
env GOCACHE=/tmp/spacechat-go-cache GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/spacechat-verify-final.VeyMq4/spacechat-client-windows-amd64.exe ./cmd/xchat
env GOCACHE=/tmp/spacechat-go-cache GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/spacechat-verify-final.VeyMq4/spacechat-linux-amd64 ./cmd/spacechat
env GOCACHE=/tmp/spacechat-go-cache GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/spacechat-verify-final.VeyMq4/spacechat-client-linux-amd64 ./cmd/xchat
env GOCACHE=/tmp/spacechat-go-cache GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/spacechat-verify-final.VeyMq4/spacechat-server-linux-amd64 ./cmd/xchat-server
```

生成的 5 个文件：

```text
spacechat-client-linux-amd64       12705415 bytes
spacechat-client-windows-amd64.exe 13056000 bytes
spacechat-linux-amd64               6132250 bytes
spacechat-server-linux-amd64       16505356 bytes
spacechat-windows-amd64.exe         6435328 bytes
```

## 未运行项

- 当前环境没有 Windows，也没有配置 `XCHAT_EXE`，因此没有运行需要 Windows ConPTY 的实机终端测试。
- 当前环境没有 `powershell` 或 `pwsh`，因此没有实际执行 `.ps1` 构建和安装脚本；这些脚本通过 Python 静态契约测试，脚本所引用的全部 Go 目标已完成交叉编译。
- 没有执行正式签名发布和生产部署，以避免生成密钥、发布制品或改变外部系统。
