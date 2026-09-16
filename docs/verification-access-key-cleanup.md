# 0.2.0 密钥与每日清理验收记录

日期：2026-09-16，时区 Asia/Shanghai。开发分支：codex/access-key-daily-cleanup；不合并 main。

## 功能与交付

- 客户端新增昵称/密钥两项，Tab 切换、Enter 继续或进入；密钥以星号显示。
- 服务端必须读取密钥文件；缺失/错误密钥被拒，不泄漏历史和在线列表。生产密钥单独部署，不在源码、日志、程序包中。
- 清理脚本通过受限 Unix socket 执行事务清空，更新聊天室实例 ID，在线客户端清空旧记录并继续连接；离线重连不恢复旧历史。
- Windows amd64 客户端版本 0.2.0；旧客户端无法登录新版服务。
- dist/xchat.exe、dist/xchat-windows-amd64.zip、dist/xchat-source.zip 与 Linux 服务端重新打包。

## 已执行测试

| 项目 | 结果 |
| --- | --- |
| Windows go test ./... -count=1、go vet ./... | 通过 |
| dev216 Linux go test -race ./... -count=1 | 通过 |
| 缺失/错误密钥、空配置、权限过宽配置 | 拒绝，通过 |
| 实际 Windows EXE：昵称和密钥输入、密钥不出现在终端输出、中文收发、缩放及退出 | 通过 |
| SQLite 事务清空、失败回滚、实例持久化及消息 ID 不回退 | 通过 |
| 在线、同步中和离线客户端清理同步，清理后继续发送 | 通过 |
| 清理后刷新同步中客户端的在线成员列表 | 通过 |
| 私有 socket 0600、活跃 socket/普通文件碰撞保护 | 通过 |
| 隔离真实 systemd 短时 timer 调用 cleanup.sh，清空独立房间后继续收发 | 通过 |
| 部署后的空/错误密钥测试（dev216 及 Windows 发起） | 通过 |
| 部署后的正确密钥双客户端发送确认 | 通过 |
| 公共 POST /clear-history 不存在；私有接口 GET 不执行清理 | 404 / 405，通过 |
| systemd 单元检查与北京时间日历解析 | 通过；仅有系统其他单元的历史路径警告 |

## 生产部署结果

- 服务地址：ws://192.168.33.216:18080/ws。
- xchat.service 与 xchat-cleanup.timer 均 active、enabled。
- 密钥文件：/etc/xchat/access.key，root:xchat、0640；按用户指定值配置，内容不公开。
- 管理 socket：/run/xchat/admin.sock，xchat:xchat、0600。
- 清理程序：/opt/xchat/cleanup.sh；调度单元：xchat-cleanup.timer / xchat-cleanup.service。
- 下次触发：**2026-09-17 00:00:00 Asia/Shanghai**，对应 2026-09-16 16:00:00 UTC。
- 默认 OnCalendar=*-*-* 00:00:00 Asia/Shanghai，AccuracySec=1s，Persistent=false。
- 升级前备份：/var/lib/xchat-backups/before-access-cleanup-20260916-144757.db，目录 0700、文件 0600，仅 root 可读。
- 升级前后均保留原有 47 条消息；没有提前手动清空生产聊天室。
- 生产收发验收额外写入一条标记消息：XChat v0.2.0 deployment check 2026-09-16。

## 复验

    go test ./... -count=1
    go vet ./...

Windows 真实程序验收（连接本机临时服务器，密钥是独立测试值）：

    $env:XCHAT_EXE = (Resolve-Path dist/xchat.exe).Path
    go test ./internal/tui -run TestWindowsExecutableInPseudoTerminal -count=1 -v

Linux 专用隔离 timer 验收（需要 root/systemd 权限，会短暂创建并清理命名为 xchat-cleanup-test-* 的定时器；不接触生产数据）：

    XCHAT_SYSTEMD_TEST=1 go test ./internal/client -run TestSystemdTimerClearsIsolatedRoom -count=1 -v

生产无密钥/错误密钥验收只需设置 XCHAT_SMOKE_ADDRESS；正确密钥验收还需 XCHAT_SMOKE_KEY 和 XCHAT_SMOKE_MARKER。应由受限配置读取密钥并传入子进程环境，避免放在 shell 命令参数或日志。TestDeploymentSmoke 会写入一条标记消息。

## 边界与注意事项

- 未等待次日真实午夜；已验证计划解析、已启用 timer 和隔离环境中真实定时触发。
- 未重启 dev216 主机；只重启了 XChat 服务完成升级。
- 定时器不补跑停机期间错过的任务，避免服务恢复时意外删除新消息。
- 每日清理是逻辑删除，不删除数据库文件、不安全擦除物理介质，也不删除管理员备份。
- ws 仍为内网明文，共享密钥不是个人身份认证或传输加密。
- 已验证 Windows ConPTY 字符输入；未逐一人工验证每种输入法候选窗口与终端字体。
