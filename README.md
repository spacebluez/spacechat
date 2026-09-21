# SpaceChat

SpaceChat 是 Go 服务端与终端客户端组成的多房间聊天程序，支持 Windows amd64、Linux amd64，以及由当前 SpaceChat 服务端下发的客户端在线更新。

客户端完成一次用户级安装后，可以在新终端直接运行 `spacechat`。后续版本由所连接的 SpaceChat 服务端提供，不依赖其他下载地址，也不要求管理员权限。

## 客户端首次安装

首次迁移仍需向用户提供一次对应平台的安装包。示例地址 `chat.example.invalid` 仅为占位符。

Windows：解压 `spacechat-windows-amd64-0.4.0.zip`，在该目录运行：

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1 -Server wss://chat.example.invalid/ws -Version 0.4.0
```

Linux：解压 `spacechat-linux-amd64-0.4.0.zip`，在该目录运行：

```sh
sh ./install.sh 'wss://chat.example.invalid/ws' '0.4.0'
```

安装后打开新终端：

```text
spacechat --version
spacechat
```

临时连接另一台服务端时，命令行参数优先于安装配置：

```text
spacechat --server wss://other.example.invalid/ws
```

Windows 安装在 `%LOCALAPPDATA%\SpaceChat`，并只修改当前用户的 `PATH`。Linux 启动器位于 `~/.local/bin/spacechat`，配置与版本文件分别使用 XDG 配置目录和数据目录。安装器拒绝符号链接输入，先运行版本化客户端的本地自检，再原子写入配置和当前版本指针。

## 在线更新行为

每次进入聊天界面前，客户端从当前服务端的固定接口读取已签名清单：

- 已是最新版本：直接进入程序。
- 存在兼容更新：显示 `Update / Skip`；跳过只对本次启动有效。
- 当前版本低于服务端最低版本：显示 `Update / Exit`，不能跳过。
- 下载、摘要校验、自检或启动新版失败：保留旧版本；可选更新可继续使用，强制更新只能重试或退出。

更新制品只从 `spacechat --server` 或安装配置选定的服务端下载。客户端拒绝跨主机重定向，并验证 Ed25519 清单签名、文件名、大小和 SHA-256。新版安装到独立版本目录，自检通过后才原子切换 `current`；新版无法启动时恢复旧指针。

WebSocket 加入请求会同时上报版本与平台。服务端在读取房间、历史和成员信息前执行最低版本检查，因此直接运行旧版本化程序也不能绕过强制更新。

## 聊天使用

输入昵称后按 Enter，再输入房间口令按 Enter。相同口令进入相同房间，不存在则自动创建。口令精确匹配且区分大小写与空格，长度为 1–256 个字符；不同口令就是不同房间。

| 操作 | 快捷键 |
| --- | --- |
| 昵称 / 口令切换 | Tab / Shift+Tab |
| 进入 / 发送消息 | Enter |
| 换行 | Shift+Enter；Ctrl+J 兼容换行 |
| 粘贴多行草稿 | Ctrl+V（推荐）或 Shift+Insert |
| 浏览历史 | PgUp / PgDn |
| 回到最新 / 已加载历史顶部 | Ctrl+End / Ctrl+Home |
| 切换房间 | F2；Enter 确认、Esc 取消 |
| 退出 | Ctrl+C |

断线后客户端自动重连并按房间补齐消息。发送结果未知时不会自动重发，避免重复消息。F2 切房会先建立目标连接并同步历史，成功后才退出原房间；失败、昵称占用或超时均保留原房间与草稿。

## 签名构建与发布

需要 Go 1.26 或更高版本和 PowerShell。正式清单必须显式提供 Ed25519 私钥。下面用 OpenSSL 生成一次 32 字节随机种子；该文件必须离线保管，不能提交仓库、复制到服务端或发给客户端：

```sh
umask 077
openssl rand -base64 32 > spacechat-release.key
```

仅构建客户端、安装包与签名更新目录：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build-client.ps1 `
  -Server wss://chat.example.invalid/ws `
  -Version 0.4.0 `
  -MinimumVersion 0.0.0 `
  -SigningKey .\spacechat-release.key
```

完整构建客户端与 Linux 服务端包：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build.ps1 `
  -Server wss://chat.example.invalid/ws `
  -Version 0.4.0 `
  -MinimumVersion 0.0.0 `
  -SigningKey .\spacechat-release.key
```

`dist` 中会生成两个平台的稳定启动器、版本化客户端、一次性安装包、签名更新目录以及服务端包。构建把发布公钥嵌入客户端，并把同一公钥随服务端包发布；构建日志不会输出私钥。

也可以单独检查公钥或生成清单：

```text
go run ./cmd/spacechat-release public-key -private-key ./spacechat-release.key
go run ./cmd/spacechat-release manifest -version 0.4.0 -minimum 0.3.2 -private-key ./spacechat-release.key -windows ./client.exe -linux ./client -out ./updates-0.4.0
```

## 服务端部署

解压完整服务端包后，以 root 运行其中的安装脚本：

```sh
sh install.sh '127.0.0.1:18081' '127.0.0.0/8'
```

参数分别是监听地址和允许访问的 CIDR 列表，示例值不是生产配置。安装依赖 Linux systemd、Python 3.7+ 和标准账户/文件管理命令。

| 项目 | 路径 |
| --- | --- |
| systemd 服务 | `xchat-rooms.service` |
| 服务端程序 | `/opt/xchat-rooms/xchat-server` |
| 已验证更新目录 | `/opt/xchat-rooms/updates` |
| 更新公钥 | `/etc/xchat-rooms/update-public.key` |
| 数据库与主密钥 | `/var/lib/xchat-rooms/rooms.db`、`/etc/xchat-rooms/encryption.key` |
| 管理 socket | `/run/xchat-rooms/admin.sock` |

安装器先复制到 `updates.new`，拒绝任何符号链接，再调用新服务端二进制的同一套生产校验逻辑验证签名、版本、摘要和文件大小。只有验证成功才切换更新目录、公钥、程序和 systemd 单元；服务重启失败时恢复上一套内容。

常用检查：

```text
systemctl status xchat-rooms --no-pager
journalctl -u xchat-rooms -n 50 --no-pager
curl http://127.0.0.1:18081/healthz
```

## 最低版本迁移与回退

首次启用在线更新时按以下顺序发布：

1. 使用 `MinimumVersion=0.0.0` 部署新服务端和清单，旧客户端暂时仍可连接。
2. 向存量用户提供最后一次人工安装包，使其切换到 `spacechat` 稳定入口。
3. 确认迁移完成后，再发布将最低版本提高到首个在线更新客户端版本的完整服务端包。

兼容发版只提高 `Version`；存在协议不兼容时才同步提高 `MinimumVersion`。最低版本不得高于清单最新版本。

回退时重新部署上一份完整服务端包，必须同时回退服务端二进制、签名更新目录、公钥和最低版本，不能只替换单个文件。已经更新到更高版本的客户端会把较旧清单视为服务端回退，不会自动降级；只要仍满足回退清单的最低版本即可继续连接。不要手工修改 `manifest.json`，任何字节变化都会使签名失效。

## 存储、安全与清理

- 昵称和正文写入 SQLite 前使用 AES-256-GCM；口令通过独立派生密钥的 HMAC-SHA256 得到房间标识，不保存原文。
- 数据库主密钥位于 `/etc/xchat-rooms/encryption.key`，安装时仅在没有既有数据库时生成。丢失主密钥无法恢复聊天内容。
- 签名私钥与数据库主密钥是两类不同密钥；前者不得部署，后者不得随发布包分发。
- `ws` 与 `wss` 均受支持；需要传输加密时由部署环境提供 TLS，并使用 `wss` 地址。
- 服务端仍可解密聊天内容；本项目不是端到端加密系统，也不提供个人账号、口令找回、私聊或文件传输。

每天北京时间 00:00，`xchat-rooms-cleanup.timer` 事务性清空全部房间历史。手动清理：

```text
sudo python3 /opt/xchat-rooms/clear_history.py
sudo python3 /opt/xchat-rooms/clear_history.py --yes
```

运行中不要单独复制 SQLite 数据库；应停止新实例后成套备份数据目录与主密钥。部署和清理脚本只操作 `xchat-rooms` 实例。

## 验证

```text
go test ./... -count=1
go vet ./...
python3 -m unittest discover -s deploy -p 'test_*.py'
```

Linux 可额外运行 `go test -race ./...`。主要边界：`internal/update` 负责签名、下载和事务更新，`internal/server` 负责更新目录及版本门槛，`cmd/spacechat` 是稳定入口，`cmd/xchat` 是可更新的版本化客户端。
