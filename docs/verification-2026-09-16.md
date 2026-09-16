# XChat 验收记录

日期：2026-09-16（Asia/Shanghai）。

## 交付与部署

- 完整源码保存在当前 xchat 仓库，另有 dist/xchat-source.zip。
- Windows amd64 客户端版本 0.1.0，默认连接 ws://192.168.33.216:18080/ws。
- Linux amd64 服务端已安装到 dev216（192.168.33.216，Kylin V10）。
- systemd 服务 xchat：active、enabled，以 xchat 非 root 用户运行。
- 程序 /opt/xchat/xchat-server；持久化数据库 /var/lib/xchat/chat.db；配置 /etc/xchat/server.env。
- 监听 192.168.33.216:18080，允许来源 192.168.0.0/16、127.0.0.0/8。
- 原主机 firewalld 未运行，INPUT 默认 ACCEPT；未修改防火墙。限制来自指定内网监听地址与应用 CIDR 白名单，不宣称新增了防火墙规则。

## 已执行验证

| 验证 | 结果 |
| --- | --- |
| Windows Go 单元/集成测试，go test ./... | 通过 |
| go vet ./... | 通过 |
| dev216 临时源码目录，go test -race ./... -count=1 | 通过 |
| Windows 与 Linux amd64 构建 | 通过 |
| dist/xchat.exe --version | xchat 0.1.0 |
| SQLite 205 条历史分页与关闭重开 | 通过 |
| 在线昵称冲突、保存后广播、无效消息拒绝、磁盘失败注入 | 通过 |
| 同步期间新增消息衔接、并发发送顺序、慢客户端隔离 | 通过 |
| 服务重启后自动补齐 205 条离线消息 | 通过 |
| 100 条最大长度中文消息的单页传输 | 通过 |
| 24/35/80/100/140 列布局与向上翻页位置保持 | 通过 |
| Windows ConPTY 运行实际 xchat.exe，昵称输入、中文发送并保存、缩放、翻页键、Ctrl+C 退出 | 通过 |
| 本机访问 dev216 /healthz | HTTP 200 |
| 两个真实网络客户端连接 dev216，发送中文并确认 | 通过 |
| 仅重启 xchat.service 后重新连接，读取已保存验收消息 | 通过 |
| systemctl is-enabled xchat | enabled |

部署验收在公共历史中留下了一条明确标记的消息：“XChat 部署验收：中文消息收发与重启持久化验证（2026-09-16）”。没有删除聊天室数据。

## 复验方法

运行自动测试：

    go test ./... -count=1
    go vet ./...

Windows 打包程序伪终端验收（本机临时服务器，不写入部署聊天室）：

    $env:XCHAT_EXE = (Resolve-Path dist/xchat.exe).Path
    go test ./internal/tui -run TestWindowsExecutableInPseudoTerminal -count=1 -v

真实部署测试默认跳过，必须显式设置地址和标记；发送模式会向公共聊天室写入一条测试消息：

    $env:XCHAT_SMOKE_ADDRESS = 'ws://192.168.33.216:18080/ws'
    $env:XCHAT_SMOKE_MARKER = '自定义唯一的验收标记'
    go test ./internal/client -run TestDeploymentSmoke -count=1 -v

重启服务后只读核验同一标记：

    $env:XCHAT_SMOKE_MODE = 'verify'
    go test ./internal/client -run TestDeploymentSmoke -count=1 -v

## 未执行项与边界

- 未重启 dev216 主机；仅验证开机启动配置已启用，以及服务级重启成功。
- Windows 测试通过真实 ConPTY 注入中文字符，未逐一人工验证不同输入法候选窗口、终端字体或同事电脑。
- 没有进行大规模用户容量或长时间压力测试。
- 无身份认证、默认 ws 明文，历史对允许网段的连接者可见；只用于可信公司内网。
- 未创建 Git 提交或新分支。
