# Access Key and Daily Cleanup Implementation Plan

> **For agentic workers:** Use executing-plans task-by-task in this session. No delegation. Commit and push only the requested development branch after verification.

**Goal:** 交付带共享密钥与每日零点清理的新客户端和服务端。

**Architecture:** join 前置密钥校验；清理事务与广播在聊天锁下执行，以 history_cleared 同步客户端。私有 Unix socket 承接清理脚本，systemd timer 调度。

**Tech Stack:** 现有 Go/WebSocket/SQLite/Bubble Tea，标准库 HTTP/Unix socket，systemd/curl；不新增第三方依赖。

## Global Constraints
- 工作分支 codex/access-key-daily-cleanup，不修改 main。
- 生产密钥只配置于 dev216 受限文件，不写源码、包、文档或日志。
- 每天北京时间 00:00 全部清空，可配置时间；不重启服务，不直接删除数据库文件。
- 输出源码、Windows amd64 客户端、Linux amd64 服务端；通过 dev216 推送。

## Task 1: Authentication
Files: internal/protocol/protocol.go; internal/server/server.go, auth_test.go; internal/accesskey/key.go, key_test.go; internal/client/client.go; internal/tui/model.go, view.go, auth_test.go; cmd/xchat-server/main.go.
Interfaces: server.New(repository,key); accesskey.Read(path); Client.Run(ctx,name,key); Join.AccessKey.
- [x] 先写无密钥/错误密钥、配置缺失、掩码显示测试，运行并确认失败。
- [x] 实现前置认证、受限文件读取、输入页和重连验证，更新所有原有测试夹具使用非生产测试密钥。
- [x] go test ./internal/accesskey ./internal/server ./internal/client ./internal/tui。

## Task 2: Online cleanup
Files: internal/store/store.go, cleanup_test.go; internal/server/cleanup.go, cleanup_test.go; internal/protocol/protocol.go; internal/client/client.go, cleanup_test.go; internal/tui/model.go, cleanup_test.go.
Interfaces: Store.Clear() (int64,error); Server.ClearHistory() (protocol.Cleared,error); Cleared{InstanceID,Deleted,CreatedAt}.
- [x] 先测试事务清空/ID 保留/失败回滚/在线事件/同步竞争，观察失败。
- [x] 实现事务、事件排序、客户端游标和视图重置，保持连接及草稿。
- [x] go test ./internal/store ./internal/server ./internal/client ./internal/tui。

## Task 3: Admin and scheduling
Files: internal/admin/admin.go, admin_test.go; cmd/xchat-server/main.go; deploy/cleanup.sh, xchat-cleanup.service, xchat-cleanup.timer, xchat.service, install.sh; scripts/build.ps1; README.md.
Interfaces: admin.Start(path,cleaner) returns closable Unix server; POST /clear-history only on private socket.
- [x] 先测试 POST 调用及失败返回、公网路由不可用，观察失败。
- [x] 实现受限 socket、脚本、timer 和安装升级（不替换已有密钥或清理数据）。
- [x] go test ./...，go vet ./...；Linux 竞态与 systemd 单元校验；隔离定时器真实触发。

## Task 4: Release
Files: docs/verification-access-key-cleanup.md; dist/ artifacts.
- [x] 构建 v0.2.0 客户端与服务端，运行实际 Windows EXE 昵称/密钥/中文测试。
- [x] 部署配置密钥并升级服务，验证缺失与错误密钥被拒、正确密钥收发、socket 清理和继续收发。
- [x] 核对北京时间零点计划和 service/timer 状态，打包源码与客户端。
- [x] 审查 diff，确认生产密钥不在跟踪文件，提交并通过 dev216 推送开发分支，核验哈希。
