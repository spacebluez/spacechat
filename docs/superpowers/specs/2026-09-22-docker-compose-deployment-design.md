# SpaceChat Docker Compose 一键部署设计

## 目标

用户从 Git 仓库获取源码后，只需执行：

```sh
docker compose up -d --build
```

即可构建并启动 SpaceChat 服务端、Windows/Linux amd64 客户端安装包、在线更新目录和每日历史清理任务。宿主机无需预装 Go、PowerShell、Python 或 systemd。

默认部署使用 `ws://`，面向可信内网并监听宿主机 `18081` 端口。正式环境可挂载 TLS 证书切换到 `wss://`；暴露到公网的部署不得继续使用默认明文配置。

## 范围与边界

- Compose 构建当前 Linux 宿主机可运行的服务端镜像，并交叉编译 Windows amd64、Linux amd64 客户端。
- 默认提供聊天、颜文字目录、客户端首装、在线更新、健康检查和每天北京时间 00:00 清理历史记录。
- SQLite 数据库及其加密密钥持久化；更新制品放在独立命名卷；管理 Unix socket 放在只供本 Compose 项目使用的运行时卷。
- 默认更新签名密钥随源码公开，目标是“开源自托管、默认可用”，不代表 SpaceChat 作者或任何第三方的发布身份认证。
- 用户可以在首次部署前挂载自己的更新私钥。更换私钥后必须重新构建嵌入对应公钥的客户端和全部签名制品。
- 数据库加密密钥不随源码提供，每个部署首次启动时独立随机生成。
- 本期不引入 Kubernetes、Swarm、自动申请公网证书、数据库集群、远程对象存储、自动备份或 ARM 客户端。

## 方案选择

采用“Compose 编排的一次性制品构建器 + 长期运行服务端 + 定时清理服务”结构。

`artifacts` 服务包含 Go 工具链、源码、固定版本的 Windows Terminal 资源和打包工具。它读取默认或用户挂载的 Ed25519 私钥，交叉编译客户端并生成签名制品，然后退出。`server` 只使用小型运行镜像，等待 `artifacts` 成功后从只读制品卷启动。`cleanup` 使用共享管理 socket 定时调用现有清理接口。

未采用以下方案：

- 在运行镜像构建阶段固定签名：更换私钥只能重新配置 Docker 构建 secret，不符合“挂载密钥后重新生成”的使用方式。
- 将预编译客户端和签名制品提交到 Git：仓库会长期保存大体积二进制，版本同步与构建可复现性较差。
- 在聊天服务运行容器内保留编译器和签名私钥：扩大了长期运行服务的攻击面，也让私钥生命周期不必要地延长。

## Compose 拓扑

根目录新增 `compose.yaml`，包含三个服务和三个命名卷：

```text
artifacts ──写入──> release-assets ──只读──> server
                                               │
data ─────────────数据库、数据库密钥───────────┤
                                               │
server ──管理 socket──> admin-runtime <── cleanup
```

### artifacts

- 使用 Dockerfile 的制品构建目标。
- Compose 只把更新签名私钥挂载给该服务；`server` 和 `cleanup` 不得看到该私钥。
- 默认私钥来自仓库中明确标为公开的 `deploy/docker/default-update-signing.seed`。
- 用户通过 `SPACECHAT_SIGNING_KEY_FILE` 指向自己的宿主机文件进行覆盖。构建器先复制到容器内权限为 `0600` 的临时文件，再调用现有发布工具，避免宿主机文件权限影响校验。
- `SPACECHAT_VERSION` 默认使用当前项目版本 `0.4.0`，`SPACECHAT_MINIMUM_VERSION` 默认 `0.0.0`。
- Docker 镜像构建阶段下载并校验固定版本的 Windows Terminal；一次性服务运行阶段不再访问外部网络。
- 在制品卷内的临时目录完成所有编译、签名和验证，通过后再原子替换 `updates`、`installers` 和 `update-public.key`。失败时不发布半成品，并以非零状态退出。
- 日志只能记录版本、公钥和输出摘要，不得输出私钥内容。

### server

- 使用不包含 Go 工具链和更新私钥的小型 Linux 运行镜像，并以固定非 root UID/GID 运行。
- 监听容器内 `0.0.0.0:18081`，Compose 默认映射宿主机 `18081`。
- 从 `release-assets` 只读加载客户端更新目录、首装目录和公钥；启动时继续验证签名、摘要、大小和文件类型。
- 将数据库与数据库加密密钥写入 `data` 命名卷。
- 将管理 Unix socket 写入 `admin-runtime` 命名卷。
- 提供 HTTP/HTTPS 健康检查；TLS 模式下只对容器本机健康探测跳过证书主机名校验，不影响客户端证书校验。
- 使用 `restart: unless-stopped`。

### cleanup

- 等待 `server` 健康后启动，与服务端使用相同的非 root UID/GID，并只挂载 `admin-runtime`。
- 扩展现有 `deploy/clear_history.py`，提供长期调度模式；按固定 `Asia/Shanghai` 时区计算下一次 00:00，届时通过 Unix socket 调用清理接口。
- 容器停机期间错过的清理不补跑，与当前 systemd timer 的 `Persistent=false` 语义一致。
- 单次清理失败时记录错误并等待下一次计划，不在短时间内自动重试破坏性操作。
- 仍保留一次性 `--yes` 调用，便于通过 `docker compose run --rm cleanup --yes` 手工清理。

## 构建与制品生成

Dockerfile 使用多阶段目标：

1. 下载 Go 模块并缓存依赖。
2. 构建 Linux 服务端和发布工具。
3. 准备跨平台客户端构建环境和经固定 SHA-256 校验的 Windows Terminal 文件。
4. 生成只包含服务端、默认颜文字目录、健康检查工具及必要 CA 的运行镜像。
5. 生成包含 `clear_history.py` 和 Python 标准库的清理镜像。

`artifacts` 运行时根据所选签名密钥导出公钥，并把该公钥通过链接参数嵌入 Windows/Linux 客户端。客户端版本、最低兼容版本、安装包清单和更新清单来自同一组环境输入，避免版本或密钥不一致。

Windows Terminal 下载发生在 `docker build`，使用现有固定版本和 SHA-256；下载或校验失败会终止镜像构建。首次构建需要访问基础镜像、Go 模块源和 Windows Terminal 发布地址，后续可使用 Docker 构建缓存。

## 动态首装地址

容器构建时无法可靠知道宿主机供客户端访问的 IP 或域名，因此首装清单增加动态地址模式，同时保留现有固定地址模式以兼容传统发布流程。

- 现有 schema 1 保持不变，固定模式继续在签名清单中保存明确的 `ws://` 或 `wss://` 地址。
- 新增 schema 2 动态模式：清单记录 `server_mode: "request"` 并省略 `server`，不伪造 localhost 地址。schema 2 只接受这一种模式，不能同时出现固定地址。
- 发布工具新增明确的动态模式参数；传统 PowerShell 发布脚本继续生成 schema 1，Docker 制品构建器生成 schema 2。
- `SPACECHAT_PUBLIC_URL` 非空时，服务端严格校验其为无凭据、无 fragment 的 `ws://` 或 `wss://` URL，并优先使用它。
- 未配置公开地址时，直接 TLS 请求根据 `request.TLS` 生成 `wss://<Host>/ws`，明文请求生成 `ws://<Host>/ws`。
- 不信任 `X-Forwarded-Proto`、`Forwarded` 等代理请求头。反向代理终止 TLS 时必须显式设置 `SPACECHAT_PUBLIC_URL=wss://域名/ws`。
- 安装包下载地址使用相同来源，将 `ws` 映射为 `http`、`wss` 映射为 `https`。
- 请求 Host 和显式公开地址都经过 URL、主机和路径校验；生成 shell 或 PowerShell 脚本时继续使用平台安全字面量转义。

示例：

```text
访问 http://192.168.1.20:18081/install/linux
安装包地址 http://192.168.1.20:18081/install/v1/packages/...
客户端地址 ws://192.168.1.20:18081/ws
```

动态模式只改变首装脚本写入的连接地址。安装包、客户端更新文件、更新清单和安装包清单仍进行 Ed25519 签名或由签名清单中的 SHA-256 约束。

## 客户端明文授权边界

当前客户端拒绝非回环的远程 `ws://`，因此一键内网部署需要把“用户从该 WS 服务执行安装”记录为对这个安装地址的明确授权，而不是全局关闭安全检查。

客户端配置扩展为：

```json
{
  "server": "ws://192.168.1.20:18081/ws",
  "allow_insecure": true
}
```

- 从 `ws://` 首装时写入 `allow_insecure: true`；从 `wss://` 首装时写入 `false`。
- 旧配置没有该字段时按 `false` 处理。
- 客户端使用安装配置中的地址时，可以读取该授权。
- 命令行显式传入另一个远程 `ws://` 地址时，仍必须同时传入 `--allow-insecure`；安装配置不得为任意临时地址提供授权。
- `wss://` 始终执行现有证书链、有效期和主机名校验，不提供跳过 TLS 校验选项。

## TLS 与公网部署

Compose 默认挂载一个仅供服务端读取的 TLS 配置目录：

- 同时存在 `tls.crt` 与 `tls.key`：服务端启用 TLS，不传入 `-allow-insecure`。
- 两者都不存在：服务端显式传入 `-allow-insecure`，以 WS 模式启动。
- 只存在其中一个：入口脚本报错并停止，不回退到明文。

公开 CA 签发的证书无需修改客户端。使用内部 CA 时，通过 `SPACECHAT_CLIENT_CA_FILE` 把公开 CA 文件只挂载给 `artifacts`，构建器将其嵌入客户端；TLS 私钥不会进入制品构建器。

公网部署文档必须要求：

1. 挂载完整 TLS 证书链和私钥；
2. 设置 `SPACECHAT_PUBLIC_URL=wss://公开域名/ws`；
3. 将 `SPACECHAT_ALLOW_CIDR` 设置为实际允许的来源范围；
4. 使用自有更新签名密钥，并重新生成客户端与制品；
5. 通过防火墙或反向代理限制不需要的端口。

默认 Compose 映射到宿主机所有接口，以满足可信局域网直接访问。README 必须在启动命令旁明确提示：如果宿主机端口可从公网访问，先完成 WSS 和访问控制配置，不能直接使用默认 WS。

## 密钥与持久化

### 更新签名密钥

默认 Ed25519 seed 是公开开发材料。任何人都可以用它签名，因此客户端验证成功只表示制品与当前自托管部署的默认构建一致，不代表发布者身份。

自有私钥只挂载到 `artifacts`。对应公钥会写入共享制品卷并嵌入客户端。更换私钥、版本或客户端 CA 时，安全的更新流程是：

```sh
docker compose down
docker compose up -d --build
```

不带 `-v` 的 `down` 保留数据库和制品命名卷。重新启动时 `artifacts` 先完成原子重建，随后服务端加载新制品。旧客户端不信任新公钥，因此更换签名密钥后必须重新下载安装与新公钥匹配的客户端；系统不尝试用旧密钥远程授权新密钥。

### 数据库加密密钥

服务端增加仅由容器部署启用的显式“缺失时初始化密钥”选项：

- 密钥存在：按现有严格格式读取，不修改。
- 密钥不存在且数据库不存在：从系统安全随机源生成独立 32 字节密钥，以 base64、`0600` 和原子创建方式写入数据卷。
- 密钥不存在但数据库已存在：拒绝启动，防止生成无法解密旧数据的新密钥。
- 密钥文件格式错误或权限不安全：拒绝启动。

数据库与密钥必须成套备份。`docker compose down -v` 会删除命名卷并导致数据不可恢复，README 必须把该命令标为破坏性操作。

## 配置接口

零配置使用以下默认值：

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `SPACECHAT_PORT` | `18081` | 宿主机映射端口 |
| `SPACECHAT_VERSION` | `0.4.0` | 构建客户端和清单的版本 |
| `SPACECHAT_MINIMUM_VERSION` | `0.0.0` | 最低兼容客户端版本 |
| `SPACECHAT_ALLOW_CIDR` | 回环及 RFC1918 私网 | 允许连接的来源网段 |
| `SPACECHAT_PUBLIC_URL` | 空 | 空时从首装请求推导地址 |
| `SPACECHAT_SIGNING_KEY_FILE` | 仓库公开 seed | 更新签名私钥宿主机路径 |
| `SPACECHAT_CLIENT_CA_FILE` | 仓库中的空占位文件 | 可选客户端内部 CA 宿主机路径 |
| `SPACECHAT_TLS_DIR` | `deploy/docker/tls` | 仅挂载给服务端的证书目录 |

Compose 使用长格式只读 bind mount，并关闭自动创建缺失源路径。路径配置错误必须直接失败，不能悄悄创建目录后退回默认配置。

敏感值通过 `.env` 或 shell 环境传入路径，不把私钥正文放进 Compose YAML、镜像层或环境变量。仓库忽略 `deploy/docker/tls` 中的实际证书、用户自有签名密钥和本地 `.env`。

## 启动、升级与失败处理

首次启动流程：

1. Docker 构建制品构建器、服务端和清理镜像。
2. `artifacts` 校验输入、构建客户端、生成并验证签名制品，原子发布到制品卷后成功退出。
3. `server` 生成或读取数据库密钥，验证更新与首装目录，打开数据库并启动监听。
4. 健康检查通过后启动 `cleanup` 调度循环。

失败策略：

- 私钥无效、版本无效、客户端 CA 无效、Windows Terminal 摘要错误或制品验证失败：`artifacts` 非零退出，`server` 不启动。
- TLS 证书/私钥不成对、公开 URL 无效、CIDR 无效、数据库密钥异常或数据库打不开：`server` 非零退出，由 `unless-stopped` 保留可观察的失败状态并按 Docker 策略重试。
- 原子发布前失败保留上一套完整制品；重新部署时服务端只会加载经过本次成功构建的目录。
- 每日清理失败不使聊天服务退出，也不立即重试；下一个北京时间午夜再次尝试。
- SIGTERM 继续使用现有十秒 HTTP 优雅关闭流程；Compose 停止超时不得短于该窗口。

升级源码使用 `git pull` 后再次执行 `docker compose up -d --build`。版本或密钥发生改变时使用前述 `down` 再 `up` 流程，确保长期运行的服务端重新加载完整制品。

## 文件与代码边界

- `Dockerfile`：多阶段构建 `artifacts`、`server`、`cleanup` 目标。
- `compose.yaml`：服务依赖、端口、环境、健康检查、只读挂载和命名卷。
- `.dockerignore`：排除 Git 元数据、本地数据库、私钥、构建输出和 worktree。
- `deploy/docker/`：容器入口、制品构建脚本、健康检查脚本、默认公开 seed、空 CA 占位文件和 TLS 目录说明。
- `cmd/xchat-server`、`internal/securestore`：显式的数据库密钥首次初始化能力和公开 URL 配置。
- `internal/update`、`cmd/spacechat-release`：兼容固定地址与请求动态地址的首装清单。
- `internal/server`：安全推导首装下载来源与客户端 WebSocket 地址。
- `internal/clientconfig`、`cmd/xchat`：仅对安装配置地址生效的明文授权。
- `deploy/client/install.sh`、`deploy/client/install.ps1`：写入 `allow_insecure` 配置。
- `deploy/clear_history.py`：一次性清理与每日调度两种模式。
- `README.md`：一键部署、安装、升级、备份、密钥信任边界和公网 WSS 操作说明。

不重构聊天协议、加密数据库格式、TUI 或 systemd 部署流程。传统发布脚本继续支持显式服务端 URL 和私有签名密钥。

## 测试与验收

自动化测试至少覆盖：

- Compose 配置可解析，三个服务、依赖条件、只读挂载和命名卷符合设计。
- Docker 多阶段镜像能够构建，运行镜像不包含更新私钥或 Go 工具链。
- 动态首装地址分别从 HTTP、直接 HTTPS 和 `SPACECHAT_PUBLIC_URL` 生成正确的下载与 WebSocket 地址；恶意 Host、无效 URL 和代理头不能绕过校验。
- 固定地址的传统首装清单保持兼容；动态地址清单的签名、包摘要和平台约束保持有效。
- Linux/Windows 安装配置按 `ws`/`wss` 写入正确的 `allow_insecure`；旧配置仍可读取；未知和重复字段仍被拒绝。
- 安装配置只授权其保存的 WS 地址；命令行指定其他远程 WS 时仍需 `--allow-insecure`。
- 首次启动生成有效数据库密钥，容器重启后密钥和数据库不变；已有数据库缺失密钥时拒绝启动。
- 默认公开私钥和临时自有私钥分别生成互相匹配但彼此不同的公钥、客户端、更新清单和安装包清单。
- 无 TLS 文件时启动 WS；完整证书对启动 WSS；仅有单边证书时拒绝启动；内部 CA 能嵌入客户端。
- 健康检查在 WS/WSS 两种模式都有效。
- 清理调度正确计算北京时间下一次午夜，单次失败不立即重试，一次性手工清理仍有效。
- 全新命名卷执行 `docker compose up -d --build` 后 `/healthz`、`/install/linux`、`/install/windows` 和更新清单可访问。
- 重启 Compose 后服务恢复，数据和密钥持久；停止期间错过的清理不补跑。
- 现有全部 Go、Python、发布脚本和 systemd 部署回归测试继续通过。

人工验收至少包括：

1. 在干净 Linux Docker 主机从 Git 克隆后一条命令启动；
2. 从另一台 Linux/Windows amd64 机器通过动态首装地址安装并进入房间；
3. 使用默认公开密钥完成一次在线更新检查；
4. 更换自有签名密钥后确认旧客户端不接受新制品，新安装客户端可正常更新；
5. 用临时可信证书切换 WSS，并确认客户端执行正常证书校验；
6. 验证北京时间午夜清理或等价的受控时钟测试；
7. 备份并恢复数据库卷与数据库密钥，确认聊天历史可读取。
