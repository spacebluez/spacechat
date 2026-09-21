# 0.4.0 颜文字实现

此实现从主分支独立重写，不依赖 `feature/Monogray/emoji_add` 分支。

- `internal/kaomoji/defaults.json` 是服务端初始目录。客户端默认不加载内置目录；通过 `GET /api/kaomoji` 读取服务端配置。
- `catalog.go` 校验 JSON 版本、体积、分类、文本、关键词及重复项，解析结果独立，缓存交付时深拷贝，避免客户端和服务端间的全局状态污染。
- 服务端 `--kaomoji` / `XCHAT_KAOMOJI` 指向配置文件，GET 时重新校验并更新 ETag。无效变更保留上次有效值，首次启动配置无效则失败。安装路径为 `/etc/xchat-rooms/kaomoji.json`，升级不会覆盖已有目录。
- 客户端连接成功和打开选择器时异步 GET，沿用聊天的证书验证、网络策略和服务路径前缀。HTTP 限时 5 秒，响应限制 1 MiB；304 复用缓存，错误响应不替换缓存。换房复制缓存，旧连接的迟到结果不进入新房间。
- 客户端 `--kaomoji` 可覆盖为个人本地目录并禁用远程获取；发布包中的示例配置不会自动读取。
- `internal/tui/kaomoji.go` 管理独立的搜索输入、分类过滤及选择位置，宽窄终端使用同一套单列选择流程。
- F3 打开；关键词搜索；Tab / Shift+Tab 切换分类；↑↓ / PgUp / PgDn 浏览；Enter 插入草稿光标位置；Ctrl+S 发送选中项并保留草稿；Esc 关闭。
- 插入前按 Unicode 字符数校验总长度，避免表情或成员名被截断。发送走普通消息确认流程；失败和断线不会自动重发。
- 选择器打开时草稿失去焦点，迟到的聊天粘贴结果被忽略；搜索键盘输入和 Enter 不会误发原草稿。

## 表情选型（2026-09-21）

初始目录整理为 10 类 72 项：软萌、开心、问候、贴贴、害羞、元气、委屈、搞怪、经典、日常。保留三种风格，优先短小、轮廓清楚的单行表情；中文搜索词覆盖“乖巧”“捂脸”“贴贴”“掀桌”“躺平”等聊天用语，同时保留英文关键词。

选型参考 [Kaomojis](https://kaomojis.jp/en)、[顔文字屋的开心分类](https://www.kaomojiya.org/yorokobu-kaomoji)、[Kaomoji Garden 的害羞分类](https://www.pocketool.app/kaomoji/en/shy)、[CopyChars](https://www.copychars.com/kaomoji) 和 [Kaomoji 的情绪及动作分类](https://kaomoji.you/en/)。结合常见字符表情进行挑选、精简与组合，分类顺序和搜索词独立整理，没有导入第三方程序或整库。

验证：`go test ./internal/kaomoji ./internal/server ./internal/client ./internal/tui`，覆盖 GET、ETag、配置热更新、错误目录回退、TLS、重定向拒绝、体积限制、换房缓存、异步选择器与草稿保留。Windows 可设置 `XCHAT_EXE` 为已构建 exe 的绝对路径，再运行 `go test ./internal/tui -run TestWindowsExecutableInPseudoTerminal -count=1 -v`，覆盖真实终端中的远程目录搜索、单独发送和草稿保留。

## Windows 字形

UTF-8 正确传输不代表终端字体包含对应字形。传统控制台的新宋体缺少多种颜文字字符，控制台缓冲区的文本断言无法发现屏幕上的缺字方框。Windows 发布包因此包含独立 Windows Terminal 和显式后备字体；双击无参数、独占可见控制台时才移交，已有终端或命令行参数启动保留当前宿主。`WT_SESSION` 防止重复启动，配置和状态存放在包内，不修改系统默认终端。

发行包来源：[微软官方便携版说明](https://learn.microsoft.com/en-us/windows/terminal/distributions)；多字体查找机制：[官方字体回退说明](https://devblogs.microsoft.com/commandline/windows-terminal-preview-1-21-release/)。固定版本、摘要校验和许可文件由 `scripts/package-terminal.ps1` 管理。字体配置覆盖本机初始目录所有字符；服务端后续增加新字符时仍需检查实际字形。

设置 `XCHAT_UNICODE_EXE` 为独立测试目录中、已内置可用 TLS 地址和信任证书的客户端完整路径，运行 `go test ./internal/tui -run TestPackagedUnicodeHost -count=1 -v`，验证双击移交、HTTPS 目录获取、搜索与发送。可通过 `XCHAT_CAPTURE_SCRIPT` 和 `XCHAT_CAPTURE_IMAGE` 指定只截取测试终端窗口的 PowerShell 脚本和输出图片，再人工检查字形；脚本接收 `-TerminalPath` 和 `-ImagePath`。测试拒绝复用已运行的同路径客户端。
