# Docker Compose 部署验证（2026-09-23）

## 实现范围

在 `codex/docker-compose-deployment` 分支的 `22d1fb2` 基础上，补齐计划 Tasks 7–9：

- `Dockerfile` 提供 `artifacts`、`server`、`cleanup` 三个目标；制品镜像缓存 Go 依赖，运行时断网编译；Windows Terminal 使用固定版本和 SHA-256。
- `compose.yaml` 编排一次性制品生成、服务端和每日清理，分别挂载数据、制品和管理 socket 卷。签名私钥与客户端 CA 只挂载给制品服务，TLS 目录只挂载给服务端。
- 服务端准备卷权限后降权到 UID/GID 10001；TLS 文件复制到受限目录，缺少证书或私钥之一时拒绝启动；健康探测按实际 TLS 副本选择协议。
- README 和 `.env.example` 覆盖首装、自有签名密钥、WSS、内部 CA、升级、备份恢复及手动清理。
- 修复每日清理等待秒数的截断问题，防止午夜前提前执行并在失败后一秒重试。
- 构建支持 `GOPROXY` 和 `WINDOWS_TERMINAL_URL`；下载源覆盖不改变 Terminal 校验值。下载增加超时和重试，安装器及文档变更可复用下载层。

## 已安装环境

Windows 10 22H2（19045.6466）重启后已成功启用 WSL2，并导入专用发行版 `SpaceChat-Docker`：

| 项目 | 实测版本 / 状态 |
| --- | --- |
| WSL / Linux 内核 | 2.7.14.0 / 6.18.33.2-microsoft-standard-WSL2 |
| Ubuntu | 24.04.5 LTS，systemd 为 PID 1 |
| Docker Engine | 29.1.3，客户端和服务端均正常 |
| Docker Compose | 2.40.3+ds1-0ubuntu1~24.04.1 |
| Buildx | 0.30.1 |
| Linux Go / Python | 1.26.0 / 3.12.3 |
| Docker 服务 | `active`，已启用随发行版启动 |

Docker 官方下载端点连接被重置，因此引擎、Compose 和 Buildx 从 Ubuntu 签名软件源安装。WSL 安装包验证了微软数字签名和官方 SHA-256；Ubuntu 镜像与 Go 工具链也核对了官方 SHA-256。未安装 Docker Desktop，未修改系统代理或 DNS。

PowerShell 可直接使用：

```powershell
wsl -d SpaceChat-Docker -- docker version
wsl -d SpaceChat-Docker -- docker compose version
wsl -d SpaceChat-Docker
```

Linux 发行版存储在 `%LOCALAPPDATA%\SpaceChatDocker`。当前工作树复制到 Linux 文件系统 `/root/spacechat-validation` 后执行验收，以保证 Unix 文件权限和 socket 行为真实有效。原工作区仍是 `D:\Github\spacechat`。

## 验证结果

| 检查 | 结果 |
| --- | --- |
| `go test ./... -count=1` | 18 个包全部通过 |
| `go vet ./...` | 通过 |
| `python3 -m unittest discover -s deploy -p 'test_*.py'` | 37 项全部通过 |
| Compose 配置专项复验 | 3 项通过，包括新增构建源覆盖 |
| `sh deploy/docker/test-images.sh` | 三个镜像构建及行为检查全部通过 |
| `sh deploy/docker/smoke-test.sh` | WS 模式通过，退出码 0 |
| `sh deploy/docker/smoke-test.sh --tls` | WSS 模式通过，退出码 0 |
| Linux amd64 客户端首装 | 两种模式均实际安装，启动器报告 `spacechat 0.4.0` |
| 测试资源清理 | 两种测试项目的容器、网络和卷已删除；保留构建缓存和镜像 |

镜像检查覆盖非 root 运行身份、运行镜像不含 Go 工具链或签名私钥、TLS 单边文件拒绝启动、清理命令、构建上下文排除私钥，以及断网环境下两个客户端平台的依赖解析。

两种冒烟模式覆盖健康检查、动态安装地址、签名清单接口、UID、密钥权限与重启保留、客户端首装和手动清理。TLS 模式还覆盖内部 CA、自有 seed、公开 URL 覆盖及缺失 TLS 私钥拒绝启动。冒烟脚本使用独立随机项目和空环境文件；仅构建下载源允许沿用环境覆盖。

验收期间修正了三个测试问题：公开 seed 测试先复制为 0600 文件以满足生产签名工具的权限约束；进程身份检查使用支持进程参数的 `docker top`；随机端口分配后重建服务固定端口，避免 Docker 重启时重新分配端口导致后续检查访问旧地址。

## 下载条件

Docker Hub 和 Go 公共下载源在本机超时，GitHub 大文件下载也出现停滞。本次使用以下内容完成真实构建：

- 基础镜像从 [AWS 提供的 Docker 官方镜像仓库](https://aws.amazon.com/blogs/containers/docker-official-images-now-available-on-amazon-elastic-container-registry-public/) `public.ecr.aws/docker/library/` 下载，再标记为 Dockerfile 原有镜像名。
- Go 模块复用本机缓存，经 `go.sum` 校验后由绑定 Docker 网桥地址的临时 HTTP 服务提供给构建器。
- Windows Terminal 首轮镜像构建直接从 GitHub 下载并通过固定 SHA-256；最终构建使用本机相同校验值的缓存 ZIP。
- 两个缓存 HTTP 服务仅用于本次构建，验收结束后关闭；镜像运行阶段不依赖它们。

实际基础镜像摘要：

| 镜像 | SHA-256 |
| --- | --- |
| `golang:1.26.0-bookworm` | `2a0ba12e116687098780d3ce700f9ce3cb340783779646aafbabed748fa6677c` |
| `alpine:3.22` | `5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8` |
| `python:3.13-alpine3.22` | `e81548ac35b07a3bd4805f275107592ef458b1e893c0e04d45aedaa19416cca5` |

因此本次确认的是容器构建和运行行为；不代表本机目前能够从所有默认公网下载地址完成无缓存构建。重新构建时可按 README 配置可访问的下载源。

## 镜像源切换复验

云服务器在 Docker Hub 元数据请求阶段超时后，新增 `SPACECHAT_BASE_IMAGE_PREFIX`，统一控制三个基础镜像的仓库前缀。当次变更默认 `docker.io/library`；可设置为 `public.ecr.aws/docker/library`，直接由 Compose 解析 ECR 地址。

- Compose 配置测试 3 项通过，覆盖默认前缀、三个服务的前缀覆盖和构建参数不进入运行环境。
- 在本机 WSL 执行 `docker compose build --pull`，三个 ECR 元数据请求均成功，解析到上表中的相同摘要；三个目标构建通过。
- `test-images.sh` 使用相同 ECR 前缀运行，镜像行为检查通过。
- Go 模块和 Terminal 下载仍复用已校验的构建缓存；本次未在用户云服务器执行，也不代表其到 ECR 的连接已确认正常。

本次复验日志为 `.cache/docker-validation/spacechat-ecr-build.log` 和 `spacechat-ecr-images.log`。

## 公网默认配置复验

后续将云端验证过的部署配置同步到仓库：

- Compose、容器入口和 `.env.example` 默认允许 `0.0.0.0/0,::/0`，公网安装请求不再被默认内网白名单拒绝。客户端地址按实际安装请求的 IP/主机与端口生成，仍支持显式白名单和公开 URL 覆盖。
- 默认使用 DaoCloud 基础镜像及 GitHub 文件镜像、`goproxy.cn` 和阿里云 Debian 镜像。新增 `DEBIAN_MIRROR` 构建参数，保留原有 Debian suite、签名密钥和软件包签名校验；Windows Terminal 的版本与固定 SHA-256 不变。
- 文档明确 HTTP/WS 默认行为、TLS 配置、旧 `.env` 的覆盖优先级，以及更新容器环境变量需要重建容器。Linux 首装测试在构建前检查所需的解压工具。

| 检查 | 结果 |
| --- | --- |
| WSL `go test ./internal/server -count=1` | 通过，含公网 IPv4、IPv6 和 IPv4-mapped IPv6 访问回归 |
| WSL 部署脚本测试 | 39 项通过 |
| 新增 Debian 源参数后的 Compose 配置复验 | 5 项通过，含示例配置一致性和自定义覆盖 |
| 阿里云 `test-images.sh` | 三个目标构建及镜像行为检查通过 |
| 阿里云 `smoke-test.sh` | HTTP/WS 模式通过，退出码 0 |
| 阿里云 `smoke-test.sh --tls` | HTTPS/WSS 模式通过，退出码 0 |
| Linux 客户端首装 | 两种模式均实际安装，启动器报告 `spacechat 0.4.0` |

容器验收使用空环境文件、独立随机项目、临时端口和临时数据卷，结束后自动清理。测试机补装了 Linux 客户端需要的 `unzip`。阿里云 Debian 镜像的约 9 MB 索引本次下载用时 39 秒；此前同机直接访问 Debian 源需要数分钟。这是本次实测值，不代表其他网络环境的下载速度。

本机 WSL 到 `goproxy.cn` 的连接超时，完整容器验收因此在阿里云执行；不能将上述结果解释为所有网络都能访问默认下载源。部署者仍可覆盖下载源参数。最终日志保存于 `.cache/docker-validation/spacechat-public-images-final.log`、`spacechat-public-ws-final.log` 和 `spacechat-public-tls-final.log`。

## 验证边界

此前原生 Windows 回归出现 Linux 路径、目录同步、执行权限、shell 和 Unix socket 相关失败；本次 Linux 全量回归已通过。没有将这些结果描述为原生 Windows 回归通过。

尚未进行独立 Windows/Linux 桌面客户端的人工聊天、真实在线版本升级和换密钥后重新安装、带历史消息的数据备份恢复，以及实际跨午夜等待。每日调度边界由受控时钟测试覆盖。上述人工场景不包含在本次自动化容器冒烟通过的结论内。

原始日志保存在本机忽略目录 `.cache/docker-validation/`。
