# 内置颜文字（Kaomoji）功能开发文档

- 文档版本：v0.1（草案）
- 日期：2026-09-18
- 适用范围：客户端 0.3.2 分支
- 关联目录：`internal/tui`、`internal/kaomoji`（新增）
- 状态：待评审

## 1. 背景与目标

当前 XChat 客户端只支持发送纯文本消息，聊天中缺少轻量的表情表达手段。本项目是纯终端（TUI）应用，不适合引入图片、GIF 或远程表情资源，因此选择 **内置颜文字（Kaomoji）** 作为“表情包”的等价物：

- 颜文字本身就是 Unicode 文本，天然符合现有消息模型与校验规则；
- 目录由 `config/kaomoji.json` 配置文件加载，无需网络请求、无需服务端改动；
- 通过快捷键调出选择器，用键盘选择并插入当前草稿，交互上对标“表情面板”。

本功能目标：

1. 内置一套分类清晰的颜文字目录；
2. 在聊天输入状态下用快捷键打开颜文字选择器；
3. 键盘完成分类切换、条目选择、插入/取消；
4. 插入后落在当前草稿的光标位置，不自动发送；
5. 不改动服务端、不改动通信协议，保持与旧客户端/旧服务端兼容。

## 2. 范围与非目标

### 2.1 本期范围

- 新增 `internal/kaomoji` 包，从 JSON 配置文件加载并校验分类颜文字目录。
- 客户端聊天态新增 `F3` 快捷键，打开/关闭颜文字选择器。
- 选择器支持键盘导航、预览、插入、取消。
- 插入逻辑与现有草稿编辑（`textarea.Model`）集成。
- 新增 `config/kaomoji.json` 与 `--kaomoji` 启动参数，客户端启动时加载配置文件。
- 配套单元测试、渲染测试与 README 文档更新。

### 2.2 非目标

- 图片 / GIF / 贴纸 / 远程表情包资源。
- 用户自定义颜文字、收藏、最近使用。
- 搜索、拼音/关键词检索。
- 服务端协议扩展、服务端存储变更。
- 登录页与换房弹窗内插入颜文字（仅聊天输入态可用）。
- 多标签、跨客户端同步颜文字库。

## 3. 现状与影响面

### 3.1 当前快捷键

| 操作 | 快捷键 |
| --- | --- |
| 昵称 / 口令切换 | Tab / Shift+Tab |
| 继续 / 进入 / 发送消息 | Enter |
| 换行 | Shift+Enter（Windows 原生客户端）；Ctrl+J 兼容 |
| 粘贴多行草稿 | Ctrl+V（推荐）、Shift+Insert |
| 编辑草稿 | 方向键、Home / End |
| 浏览历史 | PgUp / PgDn |
| 回到最新 / 已加载历史顶部 | Ctrl+End / Ctrl+Home |
| 切换房间 | F2 |
| 退出 | Ctrl+C |

### 3.2 受影响文件

新增：

- `internal/kaomoji/catalog.go`（目录加载、校验）
- `internal/kaomoji/catalog_test.go`
- `internal/tui/kaomoji.go`
- `internal/tui/kaomoji_view.go`
- `internal/tui/kaomoji_test.go`
- `config/kaomoji.json`

修改：

- `internal/tui/model.go`：增加选择器状态、F3 打开逻辑、按键路由。
- `internal/tui/view.go`：选择器打开时优先渲染选择器界面。
- `cmd/xchat/main.go`：新增 `--kaomoji` 参数并在启动时加载目录。
- `scripts/build.ps1`、`scripts/build-client.ps1`：发布包携带 `config/kaomoji.json`。
- `README.md`：快捷键表与功能说明。
- `docs/verification-kaomoji-*.md`：新增验收文档（实现后补）。

### 3.3 兼容性结论

- 颜文字就是普通消息正文，服务端与协议无需任何修改。
- 旧客户端收到含颜文字的消息时按普通文本显示，兼容。
- 旧服务端对含颜文字的消息照常通过 `ValidateBody` 校验，兼容。

## 4. 交互设计

### 4.1 快捷键选型

| 候选键 | 是否推荐 | 说明 |
| --- | --- | --- |
| **F3** | 推荐 | 与 F2 换房并列，好记（F2 换房、F3 表情）；F2 已在 Windows 原生终端验证可用，F3 同属 F 键，风险低；与 textarea 的 Emacs 默认键无冲突 |
| Ctrl+O | 备选 | 当前未被占用，但可发现性差，且个别终端/键盘映射可能已用 |
| Ctrl+E | 不推荐 | Bubble Tea textarea 默认将 Ctrl+E 绑定为行尾，冲突 |
| Ctrl+K | 不推荐 | 用户有“删除到行尾”的强肌肉记忆，且存在终端映射风险 |

结论：默认使用 **F3**。若后续需要可配置，再通过环境变量或启动参数扩展，本期不实现。

### 4.2 选择器界面

选择器为覆盖在聊天界面之上的面板，只接管键盘，不发送任何消息。

宽度充足（`width >= 60`）时使用左右两栏：

```
┌─ 选择颜文字 ────────────────────────────────┐
│ 分类         颜文字                           │
│ ▸ 开心        (◕‿◕✿)                        │
│   难过        (*^▽^*)                        │
│   惊讶        (⊙_⊙)                          │
│   生气        ...                            │
├──────────────────────────────────────────────┤
│ 预览：(◕‿◕✿)                                 │
│ ↑↓ 选择 · ←→ 切换分类 · Enter 插入 · Esc 取消 │
└──────────────────────────────────────────────┘
```

窄屏（`width < 60` 或 `height < 14`）退化为单列：先显示分类列表，进入后显示当前分类条目，底部保留操作提示。最小可用尺寸建议 24×10，与现有界面一致。

### 4.3 打开 / 插入 / 关闭流程

1. 用户已进入房间（`model.joined == true`），且没有换房弹窗（`model.switcher == nil`），输入框聚焦时按 `F3`。
2. 打开选择器：`model.input.Blur()`，记录草稿不变；选择器默认聚焦分类栏，选中第一分类、第一项。
3. 键盘导航选择目标颜文字，底部实时预览选中项。
4. 按 `Enter`：将选中文本插入当前草稿光标处，关闭选择器，重新聚焦输入框，不自动发送。
5. 按 `Esc`：关闭选择器，不插入，重新聚焦输入框，草稿保持原样。
6. 选择器打开时再按 `F3`：等同 Esc 关闭（可选，作为快捷关闭）。
7. 选择器打开时 `Ctrl+C`：保持全局行为，直接退出客户端。

### 4.4 选择器内键位

| 键 | 行为 |
| --- | --- |
| ↑ / ↓ | 当前栏内上移 / 下移（循环） |
| ← / → | 在“分类栏”和“条目栏”间切换焦点 |
| Tab / Shift+Tab | 同 ← / →，切换焦点 |
| PgUp / PgDn | 条目栏翻页 |
| Home / End | 条目栏跳到首个 / 末个 |
| Enter | 插入选中颜文字并关闭 |
| Esc | 取消并关闭 |
| F3 | 取消并关闭 |
| Ctrl+C | 退出客户端 |

切换分类时，条目选择重置为该分类第一项，避免越界。

### 4.5 插入语义

- 插入位置：**当前光标处**，调用 `model.input.InsertString(item.Text)`。
- 插入后：光标移动到插入文本之后，输入框保持聚焦，用户继续编辑或按 Enter 发送。
- 不自动发送：对标表情包“先插后发”，避免误发。
- 不替换整段草稿：已有草稿内容必须完整保留。

## 5. 数据设计

### 5.1 数据结构

新增包 `internal/kaomoji`，目录在运行时从 JSON 配置文件读取并校验：

`json
{
  "version": 1,
  "categories": [
    {
      "id": "happy",
      "name": "开心",
      "items": [
        { "text": "(◕‿◕✿)" },
        { "text": "(*^▽^*)" }
      ]
    }
  ]
}
``r

internal/kaomoji 负责解析并校验该文件：

`go
func Load(data []byte) error        // 解析 JSON 并校验后替换当前目录
func LoadFile(path string) error    // 读取文件并调用 Load
func Categories() []Category        // 返回当前目录的防御性副本
func ValidateCatalog() error        // 校验当前目录
``r

导出最小 API：

```go
func Categories() []Category // 返回内置目录（可返回副本，避免外部修改）
func ValidateCatalog() error // 供测试调用，校验目录约束
```

### 5.2 目录约束

`ValidateCatalog()` 必须保证：

1. 目录非空，且每个分类至少 1 项。
2. `Category.ID` 非空且全局唯一；`Category.Name` 非空。
3. 每个 `Item.Text` 为有效 UTF-8。
4. 不含控制字符（与 `protocol.ValidateBody` 一致），尤其不含 `\n`、`\r`（本期全部单行）。
5. 正文长度 ≤ 2000 个 Unicode 字符（runes）。
6. 每个分类内 `Item.Text` 去重；跨分类重复可允许，但同分类内不应重复。
7. 可选：全部文本可被现有终端渲染（人工在 Windows Terminal / ConPTY 下抽查）。

### 5.3 内置目录示例（首批建议）

| 分类 ID | 分类名 | 示例 |
| --- | --- | --- |
| happy | 开心 | `(◕‿◕✿)` `(*^▽^*)` `ヾ(≧▽≦*)o` `ヽ(✿ﾟ▽ﾟ)ノ` |
| sad | 难过 | `(╥﹏╥)` `(｡•́︿•̀｡)` `(T_T)` `(ノへ￣、)` |
| surprise | 惊讶 | `(⊙_⊙)` `Σ(っ °Д °;)っ` `(°ロ°)` `⊙０⊙` |
| angry | 生气 | `(╬ Ò﹏Ó)` `(￣^￣)ゞ` `(｀皿´)` `(¬_¬ )` |
| cute | 卖萌 | `(๑•̀ㅂ•́)و✧` `(๑¯◡¯๑)` `(｡•ᴗ•｡)` `(´• ω •)` |
| action | 动作 | `(ง •_•)ง` `(≧∇≦)ﾉ` `(￣▽￣)~*` `_(:з」∠)_` |
| greeting | 问候 | `(｡･∀･)ﾉﾞ` `(＾▽＾)/` `(´▽｀)ノ♪` |
| animal | 动物 | `(=^･ω･^=)` `(￣(工)￣)` `(・ω・)` `ʕ •ᴥ•ʔ` |

> 说明：上表仅用于文档示例，最终目录以 `config/kaomoji.json` 实际内容为准；修改配置文件即可增删条目，无需重新编译。

## 6. 架构与模块改动

### 6.1 `internal/kaomoji` 包

职责单一：只保存目录数据并提供校验，不依赖 `tui`、`protocol`，避免循环依赖。可独立测试。

- `catalog.go`：`Item`、`Category`、`Load()`、`LoadFile()`、`Categories()`、`ValidateCatalog()`。

### 6.2 `internal/tui` 改动

参照现有 `room_switch.go` / `room_switch_view.go` 的模式实现选择器状态机。

`model.go` 增加字段：

```go
type kaomojiPicker struct {
    categories []kaomoji.Category
    catIndex   int
    itemIndex  int
    focusPane  int // 0=分类栏, 1=条目栏
}
```

`Model` 增加：

```go
picker *kaomojiPicker
```

`Update` 中的按键路由顺序建议：

```go
// 1. Ctrl+C 全局退出
// 2. if model.switcher != nil -> updateRoomSwitch
// 3. if model.picker != nil -> updateKaomojiPicker
// 4. if model.joined && key == f2 -> openRoomSwitch
// 5. if model.joined && key == f3 -> openKaomojiPicker
// 6. if !model.joined -> 登录态处理
// 7. 其余 -> 输入框 Update
```

`view.go` 渲染顺序：

```go
if model.picker != nil { return model.kaomojiView() }
if model.switcher != nil { return model.roomSwitchView() }
```

新增关键方法：

```go
func (m *Model) openKaomojiPicker() tea.Cmd
func (m *Model) closeKaomojiPicker() tea.Cmd
func (m *Model) updateKaomojiPicker(msg tea.Msg) tea.Cmd
func (m *Model) insertKaomoji(item kaomoji.Item) tea.Cmd
func (m *Model) kaomojiView() string
```

### 6.3 插入实现要点

- 使用 `model.input.InsertString(item.Text)` 在当前光标处插入；实现前先确认 `bubbles/textarea v0.21.0` 的 `InsertString` 签名与边界行为。
- 插入后聚焦输入框：`model.input.Focus()`。
- `Model.Close()` 中无需额外清理 picker，但要保证 picker 打开时取消/退出不会 panic（`picker` 为 nil 判断）。

### 6.4 渲染要点

- 宽度计算一律使用 `ansi.StringWidth` / `ansi.Truncate`，不要用 `len()`，避免全角颜文字错位。
- 选中项使用现有 `accent` 样式；未选中使用默认；底部提示使用 `muted`。
- 预览行直接展示选中颜文字原文，并用 `ansi.Truncate` 防溢出。
- 窄屏模式必须保证不越界、不 panic。

## 7. 协议与校验影响

**结论：无需修改协议与服务端。**

原因：

- 颜文字作为 `Send.Body` 发送，走现有 `protocol.ValidateBody`。
- `ValidateBody` 允许任意有效 UTF-8 文本（除控制字符、全空白、>2000 字符外）。
- 目录约束已保证不含控制字符、不含换行、≤2000 字符，因此必然通过校验。
- 消息加密、房间隔离、历史分页、清理逻辑均不感知颜文字，无改动。

唯一需要测试的边界：草稿接近 2000 字符上限时，`InsertString` 的截断/拒绝行为要与 `textarea.CharLimit` 一致，避免插入后与校验规则不一致。

## 8. 实现步骤

| 步骤 | 内容 | 交付物 |
| --- | --- | --- |
| 1 | 新增 `internal/kaomoji` 目录与校验 | `catalog.go`、`catalog_test.go`，测试通过 |
| 2 | 在 `Model` 中接入选择器状态与 F3 打开 | `model.go` 改动，基础开关测试 |
| 3 | 实现选择器导航与插入/取消逻辑 | `kaomoji.go`，交互测试 |
| 4 | 实现选择器渲染（宽/窄两种布局） | `kaomoji_view.go`，渲染测试 |
| 5 | 集成 `view.go` / `Close()`，处理窄屏与退出 | `view.go`、`model.go` 收尾 |
| 6 | 补 README、验收文档 | `README.md`、`docs/verification-kaomoji-*.md` |
| 7 | 全量回归 `go test ./...`、`go vet ./...` | CI/本地全绿 |

## 9. 测试策略

### 9.1 目录测试（`internal/kaomoji/catalog_test.go`）

- 目录非空、分类非空、ID 唯一。
- 每条文本有效 UTF-8、无控制字符、无换行、≤2000 字符。
- 同分类内无重复。
- `Categories()` 返回的是副本或不可破坏目录数据的受保护形式。

### 9.2 选择器逻辑测试（`internal/tui/kaomoji_test.go`）

- 仅 `joined == true` 且 `switcher == nil` 时 F3 能打开；登录态、换房弹窗态 F3 不生效。
- 打开后草稿与光标保持不变。
- 分类/条目上下移动、循环、跨分类切换后条目重置到第一项。
- Enter 在光标处插入并关闭、聚焦输入框、不自动发送。
- Esc / F3 关闭且不插入、草稿不变。
- 空目录或单项目录下不 panic。

### 9.3 渲染测试

- 宽屏两栏、窄屏单列均能渲染。
- 全角/半角混排不越界，选中项样式正确。
- 终端最小尺寸 24×10 下显示提示而非崩溃。

### 9.4 Windows ConPTY 测试

- 复用现有 `XCHAT_EXE` 机制，验证真实 Windows 原生终端中 F3 键可送达、Enter 能插入。
- 验证与现有 F2 换房、Shift+Enter 换行、Ctrl+V 粘贴无冲突。

### 9.5 回归

- `go test ./... -count=1`
- `go vet ./...`
- 不新增协议/服务端用例，但必须保证现有服务端与存储测试全绿。

## 10. 验收标准

1. 聊天中输入 `F3` 能打开颜文字选择器；Esc 或 F3 可无副作用关闭。
2. 选择器内可切换分类并选择条目，底部实时预览。
3. Enter 后选中颜文字插入当前草稿光标处，光标位于插入内容之后，且不自动发送。
4. 登录页、换房弹窗中 F3 不打开选择器。
5. 草稿接近 2000 字符上限时行为明确，不与消息校验冲突。
6. 窄屏 24×10 下界面可用、不崩溃。
7. `internal/kaomoji` 目录通过全部校验测试。
8. `go test ./...` 与 `go vet ./...` 通过。
9. README 快捷键表与功能说明已更新，并补充对应验收文档。

## 11. 风险与边界

- **F 键兼容性**：F2 已验证，F3 大概率可用；若个别终端映射异常，回退方案为 Ctrl+O。
- **全角对齐**：必须统一使用 `ansi.StringWidth`/`ansi.Truncate`，避免 `len()` 计算宽度。
- **字符上限**：颜文字虽短，但用户可连续插入多条逼近 2000 上限，需测试 `InsertString` 在 `CharLimit=2000` 下的行为。
- **字体显示差异**：不同终端/字体对个别符号渲染可能不同，属正常，需在 Windows Terminal 与 ConPTY 抽查首批目录。
- **不自动发送**：本功能不改变发送语义，避免误发；发送仍由用户显式按 Enter 完成。
- **服务端零改动**：任何需要服务端配合的扩展（如自定义表情同步）都应另开设计，不混入本期。

## 12. 后续演进（可选，不在本期）

- 收藏 / 最近使用。
- 关键词或拼音搜索。
- 颜文字目录热加载。
- 用户自定义颜文字。
- 可选快捷键配置。
- 真图片/GIF 表情（需服务端协议与存储配合，另立方案）。
