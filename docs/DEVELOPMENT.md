# Akey-CLI Go 版正式开发文档

> 文档状态：正式规范  
> 规范版本：1.1.0
> 制定日期：2026-11-08  
> 最近修订：2026-11-09
> 适用范围：Akey-CLI Golang 重构版全部开发、评审、测试与发布活动

## 0. 文档说明

本文档为 **Akey-CLI Golang 重构版唯一权威开发规范**。

适配目标：

- 完全对齐火山 ArkCLI 的交互范式。
- 达到生产级 CLI 工具标准。
- 适配 Trae、Claude Code、Codex 等本地 AI Agent。
- 支持 MCP 服务托管、安全密钥管理、纯净环境注入。
- 支持长期迭代、编译分发及服务器、OpenWrt 部署。

所有 AI 迭代、功能新增和代码重构必须严格遵守本文档。实现与本文档冲突时，先修正实现；确需修改规范时，必须先评审并同步修订本文档，禁止以代码实现反向覆盖安全约束。

本文档中的“必须”“禁止”为强制要求，“应”为默认要求，“可”为可选能力。文档定义目标契约，不代表相关功能已经实现。

## 1. 项目定位

### 1.1 项目简介

`akey` 是自研开源、对标 ArkCLI 的安全 API Key 与 Agent 管理 CLI，核心解决以下问题：

1. 多 AI 厂商 Key 混乱管理。
2. Key 明文落地及配置泄露风险。
3. 本地 Agent（Trae 优先）MCP 配置繁琐。
4. 开发环境变量污染及 Shell 历史泄露。
5. 缺少统一的 Agent 快速配置工具。

### 1.2 核心特性

- 多 Profile 环境隔离：厂商、Key 类型、模型、Base URL、环境变量前缀及备注。
- 服务商静态预设：普通 API Key、Agent Plan、Coding Plan 使用独立 Profile、Endpoint 和密钥环项。
- 系统密钥环加密存储：零明文落地。
- 子进程纯净环境注入：不污染当前终端，不在 Shell 历史中携带密钥。
- 本地 AI Agent 一键配置：Trae 优先。
- MCP 服务动态挂载与移除。
- 交互式 Helper 向导。
- 跨平台单二进制、零第三方运行时依赖。

### 1.3 设计哲学（硬性约束）

1. 密钥永不落地明文。
2. 永不篡改当前 Shell 环境。
3. 命令交互 100% 对齐本文档定义的 ArkCLI 风格命令契约。
4. 配置与密钥严格分离。
5. 采用插件式适配器架构，新增 Agent 不修改核心模块。
6. 安全优先，体验对齐官方。

### 1.4 非目标

首个稳定版本不包含：

- 云端密钥同步。
- 自建密钥明文文件后端。
- 团队权限系统或远程账号体系。
- AI 模型推理代理服务。
- 绕过操作系统密钥环的自定义加密存储。

## 2. 技术栈（固定不变）

| 领域 | 技术选型 | 约束 |
| --- | --- | --- |
| 语言 | Go 1.23+ | `go.mod` 的 Go 版本不得低于 1.23 |
| CLI 框架 | `spf13/cobra` | 统一命令树、参数解析、帮助与错误 |
| 密钥存储 | `99designs/keyring` | 禁止自行实现系统密钥环替代品 |
| 美化输出 | `charmbracelet/lipgloss` | 仅负责样式，不参与业务逻辑 |
| TUI 交互向导 | `charmbracelet/bubbletea` | 仅用于交互流程与隐藏密钥输入 |
| 序列化 | 标准库 `encoding/json` | 配置与 JSON 输出统一使用标准库 |
| 进程与环境管理 | 标准库 `os/exec` | 禁止通过拼接 Shell 命令执行用户程序 |

未经规范评审，不得引入新的核心依赖。测试依赖不得进入发布二进制的业务依赖链。

## 3. 项目目录架构（永久固定）

```text
Akey-CLI/
├── cmd/
│   └── akey/
│       ├── main.go              # 程序入口、退出码、顶层错误输出
│       ├── root.go              # Cobra 根命令与全局 Flag
│       ├── auth.go              # auth 命令入口
│       ├── profile.go           # profile 命令入口
│       ├── run.go               # run 命令入口
│       ├── env.go               # env 命令入口
│       ├── agent.go             # agent 与 MCP 命令入口
│       ├── helper.go            # Helper TUI 入口
│       ├── prompt.go            # 终端输入、隐藏输入与确认交互
│       └── render.go            # 输出写入与 Lip Gloss 样式适配
├── internal/
│   ├── apperr/
│   │   └── error.go             # 分类错误、安全消息与退出码映射
│   ├── config/
│   │   ├── store.go             # 元数据读取、校验、原子保存
│   │   ├── types.go             # Profile、Key 类型与全局状态类型
│   │   └── presets.go           # 服务商、Endpoint 与 Key 类型静态预设
│   ├── keyring/
│   │   ├── store.go             # 密钥环接口与后端实现
│   │   └── redact.go            # 脱敏及禁止序列化保护
│   ├── envinject/
│   │   ├── inject.go            # 环境副本、覆盖与变量名生成
│   │   └── export.go            # 各 Shell 的安全转义与脚本生成
│   ├── process/
│   │   ├── run.go               # 子进程启动、IO 与退出状态
│   │   ├── signal_unix.go       # Unix 进程组与信号转发
│   │   └── signal_windows.go    # Windows 进程组与 Ctrl+Break 转发
│   ├── agent/
│   │   ├── adapter.go           # Agent Adapter 统一接口与注册表
│   │   ├── trae/
│   │   │   └── adapter.go       # Trae 适配器
│   │   ├── claudecode/
│   │   │   └── adapter.go       # Claude Code 适配器
│   │   └── codex/
│   │   │   └── adapter.go       # Codex 适配器
│   ├── mcp/
│   │   ├── manager.go           # MCP 挂载、移除与查询
│   │   └── types.go             # MCP Server 元数据及协议结构
│   ├── validate/
│   │   └── validate.go          # 名称、URL、前缀及命令校验
│   └── format/
│       ├── table.go             # 纯表格字符串渲染
│       └── json.go              # 纯 JSON 渲染
├── scripts/
│   ├── install.ps1              # Windows 免 Go 安装器
│   └── install.sh               # macOS/Linux 免 Go 安装器
├── .github/workflows/           # 三平台测试与 Release 自动化
├── .goreleaser.yaml             # 压缩包、校验和及版本注入
├── docs/
│   └── DEVELOPMENT.md           # 本文档
├── go.mod
├── go.sum
└── LICENSE
```

测试文件采用 Go 原生 `*_test.go` 形式，与被测代码同目录放置。构建脚本和发布配置可在后续新增，但不得改变上述核心分层。

### 3.1 分层原则

- `cmd` 层只负责参数解析、命令入口、终端交互、调用内部逻辑及最终输出。
- `internal` 层只负责纯逻辑，不直接打印，不发起终端交互。
- 所有文件读写、密钥操作、进程操作必须封装在 `internal` 对应模块中。
- `internal` 返回结构化结果或错误，由 `cmd` 决定表格、JSON、颜色及退出码。
- `cmd` 不得实现密钥环、配置存储、Agent 配置解析等业务规则。
- 禁止跨层绕过适配器直接修改 Agent 配置。

## 4. 数据架构（核心双分离模型）

### 4.1 元数据（明文、可查看）

默认存储路径：`~/.akey/profiles.json`。

其中 `~` 必须通过 `os.UserHomeDir()` 解析，再拼接 `.akey`，不得与 `os.UserConfigDir()` 混用。配置目录权限应设置为 `0700`，文件权限应设置为 `0600`；Windows 使用当前用户 ACL 提供等价保护。

单 Profile 结构：

```json
{
  "id": "8e2ac53fc10b4c748ef0459d0ec43d31",
  "name": "volc-prod",
  "provider": "volcengine",
  "credential_type": "api",
  "base_url": "https://ark.cn-beijing.volces.com/api/v3",
  "model": "doubao-pro-32k",
  "env_prefix": "API",
  "note": "生产环境",
  "created_at": "2026-11-08T08:00:00Z",
  "updated_at": "2026-11-08T08:00:00Z"
}
```

字段约束：

| 字段 | 类型 | 必填 | 规则 |
| --- | --- | --- | --- |
| `id` | string | 是 | 创建时使用 `crypto/rand` 生成 16 字节随机数并编码为 32 位小写十六进制；重命名时不变 |
| `name` | string | 是 | 全局唯一；匹配 `^[a-z0-9][a-z0-9_-]{0,31}$` |
| `provider` | string | 是 | 厂商标识，仅作元数据，不保存任何凭证 |
| `credential_type` | string | 否 | `api`、`agent-plan`、`coding-plan`；缺省及旧配置按 `api` 读取，创建后不可通过 `profile update` 修改 |
| `base_url` | string | 是 | 必须为绝对 `http` 或 `https` URL，不得包含用户信息、查询密钥 |
| `model` | string | 是 | 默认模型或 Endpoint 标识，由用户明确填写 |
| `env_prefix` | string | 否 | 默认 `API`；匹配 `^[A-Z_][A-Z0-9_]*$` |
| `note` | string | 否 | 普通备注，不得保存密钥或令牌 |
| `created_at` | string | 是 | UTC RFC 3339 时间 |
| `updated_at` | string | 是 | UTC RFC 3339 时间 |

全局状态与 Profile 列表保存在同一文件中：

```json
{
  "version": 1,
  "current_profile": "volc-prod",
  "profiles": [
    {
      "id": "8e2ac53fc10b4c748ef0459d0ec43d31",
      "name": "volc-prod",
      "provider": "volcengine",
      "credential_type": "api",
      "base_url": "https://ark.cn-beijing.volces.com/api/v3",
      "model": "doubao-pro-32k",
      "env_prefix": "API",
      "note": "生产环境",
      "created_at": "2026-11-08T08:00:00Z",
      "updated_at": "2026-11-08T08:00:00Z"
    }
  ]
}
```

规则：

- `version` 当前固定为 `1`；未来迁移必须向后读取或明确执行迁移。
- `current_profile` 为空表示没有默认环境。
- 删除当前 Profile 后必须清空 `current_profile`，禁止静默切换到其他环境。
- 元数据写入必须使用“同目录临时文件 + flush + 原子替换”，避免中断导致半文件。
- 多进程写入时必须串行化或使用锁文件保护，读改写不得丢失其他进程更新。
- JSON 中禁止出现 `api_key`、`apiKey`、`token`、`secret`、`password` 等凭证字段。

Provider 预设必须保存在 `internal/config/presets.go`，仅包含稳定 ID、显示名、Provider、Base URL 和 Credential Type，不得联网查询或包含凭证。普通火山方舟使用 `/api/v3`，Agent Plan 使用 `/api/plan/v3`，Coding Plan 使用 `/api/coding/v3`。三类预设必须生成不同 Profile；即使用户录入相同 Key 字符串，也不得自动共享或切换密钥环项。其他服务商可复用 `api` 类型，Custom 由用户明确填写 Base URL。

### 4.2 密钥数据（密文、系统密钥环）

存储后端自动适配：

- Windows：Windows 凭据管理器。
- macOS：Keychain。
- Linux：Secret Service。

统一键值规则：

- Service：`com.akey.cli`。
- Account：`profile:<profile.id>`。
- Value：用户 API Key 或 Plan Key，仅存在于密钥环和运行时受控内存中。

必须使用不可变 `profile.id` 关联密钥，禁止使用可修改的 Profile 名称作为密钥环主键。删除 Profile 时同步删除对应密钥；仅重命名 Profile 时不得重建或复制密钥。

Key 永远不写入 JSON、配置文件、日志、Shell 历史或命令参数，仅在运行时内存中短暂存在。读取后不得进行不必要的字符串复制、格式化或长期缓存。

系统密钥环不可用时应返回明确错误及平台处理提示，禁止自动降级为明文文件。OpenWrt 等精简系统必须提供可用的 Secret Service 兼容后端，否则密钥相关命令应安全失败。

### 4.3 安全铁律

1. 禁止通过普通输出、表格、JSON、日志或错误堆栈展示 Key。
2. 禁止日志输出 Key，调试模式也不例外。
3. 禁止修改父进程或全局环境变量。
4. 禁止通过命令行参数传递 Key，不提供 `--api-key`、`--key` 等 Flag。
5. 所有自动运行必须依赖子进程独立 Env 注入。
6. 错误返回前必须对 URL、错误文本及第三方错误进行凭证脱敏。
7. 密钥输入必须隐藏回显；不得将密钥放入 Shell 命令示例。
8. 不得生成包含 Key 的崩溃转储、审计日志、遥测数据或临时文件。

`env export` 是唯一面向用户显式 `eval` 的手动会话能力，详见 5.4。它不得被内部模块、`run`、Agent 或 MCP 隐式调用。

## 5. 完整命令树

统一调用形式：

```text
akey [全局 flags] <command> [subcommand] [flags] [args]
```

### 5.0 全局 Flag（所有命令通用）

```text
-p, --profile string    指定 Profile 名称；覆盖当前默认 Profile，但不修改默认状态
-o, --output string     输出格式：table 或 json；默认 table
    --no-color          禁用颜色及终端样式
    --non-interactive   禁止交互询问；信息不足时直接报错
    --verbose           输出不含敏感信息的诊断详情
-h, --help              查看帮助
-v, --version           查看版本（仅根命令）
```

约束：

- 全局 Flag 必须在所有子命令中保持一致语义。
- `--profile` 只选择本次命令环境，不执行 Profile 切换。
- 机器可读数据写 stdout；提示、警告和错误写 stderr。
- `--output json` 时 stdout 必须是单个合法 JSON 文档，不得混入样式字符或提示文本。
- 非交互模式不得退化为明文密钥参数输入。
- 无参数 `akey` 与 `akey helper` 必须进入同一个精简向导；`akey --help` 仍显示帮助，`akey --version` 仍显示版本。
- 无参数配合 `--non-interactive` 时必须返回使用错误，不得等待终端输入。

### 5.1 `auth` 认证模块

```text
akey auth login [--profile <name>]
akey auth logout [--profile <name>] [--yes]
akey auth status [--profile <name>]
```

| 命令 | 行为 |
| --- | --- |
| `auth login` | 通过隐藏输入设置或轮换指定 Profile 的 Key；不修改 Provider、Base URL 和 Model |
| `auth logout` | 仅删除指定 Profile 的 Key，保留元数据；交互模式需要确认 |
| `auth status` | 显示 Profile、密钥环后端及“已配置/未配置/不可用”状态，不显示 Key 或其片段 |

`auth login` 不得创建不存在的 Profile。用户应先执行 `profile create`，避免凭证归属不清。

### 5.2 `profile` 环境管理

```text
akey profile create <name> [--provider <provider>] [--credential-type <api|agent-plan|coding-plan>] [--base-url <url>] [--model <model>] [--env-prefix <prefix>] [--note <note>]
akey profile list
akey profile show [name]
akey profile update <name> [--provider <provider>] [--base-url <url>] [--model <model>] [--env-prefix <prefix>] [--note <note>]
akey profile rename <old-name> <new-name>
akey profile use <name>
akey profile current
akey profile delete <name> [--yes]
```

行为约束：

- `create` 缺少必要元数据时进入交互向导；创建元数据后引导隐藏输入 Key。`--credential-type` 缺省为 `api`，必须在读取 Key 前校验，创建后不可通过 `update` 修改。
- `update` 仅修改显式提供的字段，不触发 Key 轮换；轮换使用 `auth login`，不得修改 Credential Type。
- `list` 显示当前标记、名称、Provider、Credential Type、Model、Base URL、Key 状态和备注。
- `show` 只显示元数据和 Key 配置状态，绝不显示 Key。
- `use` 仅更新 `current_profile`。
- `current` 无默认 Profile 时返回明确错误，不静默选择第一个 Profile。
- `rename` 不改变 Profile ID，不影响密钥环数据或 Agent 中按 ID 生成的内部关联；外部命令引用名称时由适配器同步更新。同步失败时必须报告失败前已完成的变更数量，不执行跨 Agent 回滚，也不得把部分成功报告为全部成功。
- `delete` 同时删除元数据和密钥环中的 Key；先备份元数据，再删除 Key，最后提交元数据。任一阶段失败都不得假装全部成功，提交失败时应尝试恢复 Key 并明确报告补偿结果。

### 5.3 `run` 安全运行（核心能力）

```text
akey run [--profile <name>] -- <command> [args...]
```

示例：

```powershell
akey run --profile volc-prod -- python app.py
```

能力：

- 创建当前系统环境的独立副本。
- 自动注入 `<PREFIX>_KEY`、`<PREFIX>_BASE_URL`、`<PREFIX>_MODEL`。
- 默认前缀为 `API`，即 `API_KEY`、`API_BASE_URL`、`API_MODEL`。
- 不污染当前终端。
- 子进程结束后释放密钥引用，不保留全局缓存。
- 透传标准输入、输出、错误和退出状态。

文档、帮助和测试输出均不得使用会展示环境密钥的命令作为推荐示例。

### 5.4 `env` 环境导出

```text
akey env export [--profile <name>] --shell <bash|zsh|fish|powershell|cmd>
akey env unset [--profile <name>] --shell <bash|zsh|fish|powershell|cmd>
```

`env export` 输出可供用户显式 `eval` 或 `Invoke-Expression` 的 Shell 脚本，供手动终端会话使用。`env unset` 只生成清除对应三个变量的脚本，不访问密钥环。

由于父进程无法直接修改当前 Shell，`akey` 本身不得执行 `eval`，只能输出脚本。此命令是用户主动选择的手动会话例外，不是自动运行路径：

- stdout 仅包含脚本，不包含成功提示、颜色或日志。
- 密钥必须进行目标 Shell 的正确转义，禁止命令注入。
- 不得写入临时文件，不得记录日志，不得在错误信息中回显脚本。
- 帮助中必须提示：优先使用 `akey run`；`eval` 后变量会存在于当前 Shell，并可能被子进程读取，使用完毕应执行 `env unset` 生成脚本进行清理。
- `--output json` 不适用于 `env export/unset`，指定时返回用法错误，防止 Key 被结构化日志收集。
- 除该显式导出通道外，任何命令不得输出 Key。
- `--shell cmd` 面向交互式 CMD，不按批处理语义转义；值包含换行、`%` 或 `!` 时必须安全失败，不得输出可能被 CMD 展开的错误密钥。

示例只展示安全调用形式，不展示生成后的密钥内容：

```bash
eval "$(akey env export --profile volc-prod --shell bash)"
eval "$(akey env unset --shell bash)"
```

### 5.5 `agent` 本地 AI 管理

支持适配器：

- `trae`：优先并完整适配。
- `claude-code`：通过独立适配器支持。
- `codex`：通过独立适配器支持。

```text
akey agent list
akey agent detect
akey agent status [agent]
akey agent configure <agent> [--profile <name>]
akey agent mcp list <agent>
akey agent mcp add <agent> <server> [--profile <name>] [--command <command>] [--arg <arg>...]
akey agent mcp remove <agent> <server> [--yes]
```

行为约束：

- `list` 列出已注册适配器及支持状态。
- `detect` 只检测本机安装与配置路径，不修改文件。
- `status` 显示已配置的 Profile 引用、MCP 数量和配置文件路径。
- `configure` 只写元数据和运行时启动方式，密钥仍由 `akey run` 注入。
- `mcp add` 对内置 Server 使用受控模板；自定义 Server 必须显式提供启动命令。
- `mcp remove` 仅移除目标 Agent 中对应配置，不删除其他 Agent 或系统中的服务。
- 所有修改必须先解析并校验现有配置，保留未知字段，采用备份与原子替换，禁止直接覆盖整个文件。

### 5.6 默认配置向导（`akey` / `helper`）

```text
akey
akey helper
```

默认交互流程：

1. 编号选择 Provider 与 Key 类型；仅 Custom 追问并预先校验 Base URL。
2. 输入 Model/Endpoint ID，再隐藏输入 API Key、Agent Plan Key 或 Coding Plan Key。
3. 自动以预设 ID 生成 Profile 名称，重名时追加 `-2`、`-3` 等后缀；Env Prefix 固定为 `API`，备注为空。
4. 沿用 Create → Set → Use；Key 保存失败必须回滚新建 Profile，且不得触碰 Agent。
5. 复用 `agent detect` 的检测循环，以 `Installed || ConfigExists` 筛选候选 Agent。
6. 0 个候选时正常结束；1 个候选时在写入前确认一次；多个候选时允许编号单选或跳过。
7. Trae/Claude Code 根据 `ChangeSet.Changed` 报告 `configured` 或 `unchanged`；Codex no-op 必须报告 `not_required`，不得宣称写入成功。
8. 用户跳过时报告 `skipped`，无候选时报告 `not_detected`，不得无条件输出 `configured=true`。

默认向导不得包含 MCP 问答；MCP 继续使用 `agent mcp` 高级命令。当前 Agent `Configure` 只初始化 `mcpServers`，不绑定 Provider 或 Model，界面必须称为“Agent 基础初始化”，不得宣称“已接入模型”。

Helper 必须复用与其他命令相同的内部服务，禁止复制业务逻辑。Key 创建前取消不得留下配置；基础配置完成后的 Agent 阶段取消或失败时保留 Profile、Key 和默认环境，并明确报告已完成与未完成的步骤。交互提示写 stderr，JSON 结果写 stdout。

## 6. 核心模块详细规范

### 6.1 环境注入模块（`envinject`）

唯一正确流程：

1. 解析目标 Profile 并从密钥环读取 Key。
2. 读取当前系统环境副本 `os.Environ()`。
3. 从副本中移除同名旧变量，再追加以下三个变量：

   ```text
   <PREFIX>_KEY
   <PREFIX>_BASE_URL
   <PREFIX>_MODEL
   ```

4. 默认 `PREFIX=API`，对应 `API_KEY`、`API_BASE_URL`、`API_MODEL`。
5. 使用新环境启动子进程。
6. 转发 `Stdin`、`Stdout`、`Stderr`。
7. 转发中断和终止信号至子进程，并等待其退出。
8. 返回子进程退出状态并释放密钥引用。

环境变量覆盖必须处理重复项；Windows 环境变量名按不区分大小写处理，Unix 按区分大小写处理。

禁止操作：

- 禁止调用 `os.Setenv` 修改 `akey` 自身环境以实施注入。
- 禁止修改父 Shell、用户级或系统级持久环境。
- 禁止直接通过命令行参数携带密钥。
- 禁止打印环境副本或密钥内容。
- 禁止默认经 `sh -c`、`cmd /c` 或 `powershell -Command` 拼接执行命令。

自定义前缀示例：`OPENAI_API` 对应 `OPENAI_API_KEY`、`OPENAI_API_BASE_URL`、`OPENAI_API_MODEL`。单次注入只生成目标前缀对应的一组变量，不额外注入其他厂商变量。

### 6.2 子进程与退出状态

- 使用 `exec.Command` 的参数数组形式传递命令和参数，不重新拼接命令字符串。
- 子进程正常或异常退出时，`akey run` 应透传其退出码。
- 收到 Ctrl+C 时优先将中断信号发送给子进程，不得立即脱离并留下孤儿进程。
- Unix 转发 `SIGINT`、`SIGTERM`；Windows 使用控制台中断与进程终止能力实现等价行为。
- 无法取得子进程退出码时返回统一运行错误，禁止伪造成功状态。

### 6.3 Agent 适配器规范

所有 Agent 必须遵循：

1. 自动识别本机安装和配置路径。
2. 只写元数据，绝不写密钥。
3. 配置中不得出现明文 Key，也不得以 Base64 等方式变相保存 Key。
4. 真实密钥由 `akey run` 在运行时注入。
5. 写入前校验配置；写入失败时恢复原文件。
6. 保留用户已有的其他服务、字段和顺序语义，禁止无关格式化扩散。
7. 适配器不得直接调用 `keyring` 模块读取 Key。

核心接口契约：

```go
type Adapter interface {
	Name() string
	Detect() (InstallInfo, error)
	Status() (Status, error)
	Configure(profile config.Profile) (ChangeSet, error)
	AddMCP(profile config.Profile, server mcp.Server) (ChangeSet, error)
	RemoveMCP(serverName string) (ChangeSet, error)
}
```

接口参数和返回类型不得包含 API Key。新增 Agent 只需新增适配器目录并注册，不得修改 `envinject`、`keyring`、`config` 等核心模块。

#### Trae 专属适配规则

- 默认配置路径：`~/.trae/mcp.json`。
- MCP 条目只写官方支持的 `command`、`args` 等字段，不写未经验证的 `baseURL`、`model`、`provider` 字段。
- Provider、Base URL、Model 和密钥完全依赖 `akey run` 运行时注入。
- MCP 启动配置使用 `akey run --profile <name> -- <server-command>` 形式。
- 若用户已有合法配置，采用合并策略；发生冲突时明确提示，不静默覆盖。

MCP 配置示意（不包含真实服务参数）：

```json
{
  "mcpServers": {
    "web-search": {
      "command": "akey",
      "args": [
        "run",
        "--profile",
        "volc-prod",
        "--",
        "web-search-mcp"
      ]
    }
  }
}
```

Claude Code 用户配置默认使用 `~/.claude.json` 顶层 `mcpServers`。Codex 适配器必须通过官方 `codex mcp list/get/add/remove` 委托管理 `~/.codex/config.toml`，禁止自行解析或覆盖 TOML；CLI 不存在、输出不兼容或操作无法安全完成时必须明确失败。唯一例外是 Profile 重命名时 CLI 与默认配置文件均不存在，此时可将 Codex 视为无引用适配器并跳过；只要默认配置文件存在，就不得假设其中无引用，CLI 不可用仍必须明确失败。检查配置文件是否存在不属于解析 TOML。核心模块不得依赖各 Agent 的路径或文件格式，适配器实现前必须验证对应版本的真实配置契约。

### 6.4 MCP 管理器规范

首版内置受控模板：

- `web-search`
- `code-execute`
- `memory`
- 自定义 MCP Server

统一元数据：

```json
{
  "name": "web-search",
  "command": "web-search-mcp",
  "args": [],
  "profile": "volc-prod"
}
```

规则：

- Server 名称在单个 Agent 内唯一，匹配 Profile 名称规则。
- 内置模板只保存非敏感启动元数据，不在模板中内置真实 Key。
- 自定义命令使用参数数组保存，不保存拼接后的 Shell 字符串。
- JSON 导出采用 MCP `mcpServers` 对象结构，字段名称不得自行改写为其他协议。
- `mcp list --output json` 输出目标 Agent 的合法 `mcpServers` 对象；表格输出用于人工查看。
- 删除操作必须幂等提示目标不存在，不得删除同名但不受 `akey` 管理的外部配置，除非用户明确确认。

## 7. 编译与分发规范

### 7.1 单文件编译

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o akey ./cmd/akey
```

Windows PowerShell：

```powershell
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags="-s -w" -o akey.exe ./cmd/akey
Remove-Item Env:CGO_ENABLED
```

参数说明：

- `CGO_ENABLED=0`：生成静态 Go 二进制并降低跨平台动态库依赖。
- `-trimpath`：移除本机绝对构建路径。
- `-s`：去除符号表。
- `-w`：去除调试信息。
- 产出为可直接分发的单二进制；操作系统密钥环属于系统能力，不视为第三方运行时依赖。

版本信息通过独立变量注入，不得硬编码在多个文件中：

```bash
go build -trimpath -ldflags="-s -w -X main.version=1.1.0 -X main.commit=<commit> -X main.buildDate=<utc-time>" -o akey ./cmd/akey
```

### 7.2 跨平台支持

目标平台固定为 9 个 Release 构建：

- Windows：AMD64、ARM64。
- macOS：AMD64、ARM64。
- Linux：AMD64、ARM64、ARMv7。
- OpenWrt：Linux MIPS、MIPSLE（softfloat）。

每个发布目标必须在真实系统或等价测试环境验证：密钥环、信号处理、路径解析和子进程退出码。交叉编译成功不等于该平台功能验收通过。

发布前必须执行：

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

`go test -race` 在目标平台不支持时，可在受支持的 CI 平台执行，但不得跳过普通测试。

### 7.3 GoReleaser 与 Release 资产

- 使用 `.goreleaser.yaml` 统一构建上述 9 个目标，注入 `main.version`、`main.commit` 和 UTC `main.buildDate`。
- Windows 资产使用 ZIP，其余目标使用 tar.gz；所有压缩包必须列入统一 `checksums.txt`，校验算法为 SHA256。
- `scripts/install.ps1` 与 `scripts/install.sh` 必须作为 Release 额外资产发布，不得要求用户安装 Go 或 GoReleaser。
- `v*` 标签触发测试和 `.github/workflows/release.yml`，测试通过后由 GoReleaser 创建 Release；禁止覆盖既有版本或复用旧标签。
- 发布前必须执行 `goreleaser check` 和 snapshot 构建，逐一核对目标数量、资产名称、压缩格式、SHA256 与版本注入。

### 7.4 一键安装器与 CI

- Windows 默认安装到 `%LOCALAPPDATA%\Programs\Akey\bin`，幂等更新当前用户 PATH；不得修改系统 PATH 或父进程环境。
- macOS/Linux 桌面架构安装到 `$HOME/.local/bin`；目录不在 PATH 时只给出明确提示，不得批量改写 Shell 启动文件。OpenWrt/MIPS 使用手动压缩包安装。
- 安装器默认解析 `latest`，同时支持显式版本；必须下载到临时目录、验证 SHA256 后解压并替换二进制。平台不支持、下载失败或校验失败时必须清理临时文件并停止，不得留下半安装状态。
- 安装器不得接收、读取或输出 API Key/Plan Key，也不得将密钥放入 URL、命令参数、日志或环境变量。
- `.github/workflows/test.yml` 必须在 Windows、Ubuntu 和 macOS 运行 `go test ./...`；Ubuntu 额外执行 `go vet ./...` 和支持的竞态检测。
- Release 工作流必须对安装脚本执行三平台 smoke test，覆盖安装路径、PATH 行为及 `akey --version`；脚本语法检查不能替代真实安装验证。

## 8. 日志与输出规范

统一 UI 范式：

- `✓` 或 `✅`：成功提示。
- `✗` 或 `❌`：错误提示。
- `ℹ` 或 `ℹ️`：信息提示。
- 表格格式化列表。
- 错误统一捕获、统一输出，不向普通用户抛原生堆栈。

具体约束：

- 默认启用简洁模式；`--verbose` 只增加安全诊断，不增加密钥、完整环境或敏感请求内容。
- 颜色仅用于增强语义，不得作为区分状态的唯一方式。
- 非 TTY 输出自动禁用颜色和动态光标；`--no-color` 强制禁用。
- 表格无数据时显示明确空状态，不输出伪造表头数据。
- JSON 字段使用稳定的小写 snake_case。
- 错误信息必须包含“发生了什么”和“下一步如何处理”，但不得包含凭证。
- Cobra 的原生用法输出与业务错误应避免无意义重复；参数错误可显示简短帮助。

### 8.1 退出码

| 退出码 | 含义 |
| --- | --- |
| `0` | 成功 |
| `1` | 未分类运行错误 |
| `2` | 命令或参数用法错误 |
| `3` | 认证、密钥不存在或密钥环访问失败 |
| `4` | Profile、Agent 或 MCP 目标不存在 |
| `5` | 状态冲突或用户取消确认 |
| `6` | 配置读写、校验或迁移失败 |
| `130` | 用户中断 |

`akey run` 优先透传子进程退出码；若与上述内部退出码冲突，以子进程结果为准。JSON 错误输出至少包含 `error.code`、`error.message` 和可选的 `error.hint`，不得包含堆栈或敏感详情。

## 9. 测试与安全验收规范

每个核心模块必须具备单元测试，以下场景属于发布阻断项：

1. `profiles.json` 在任何操作后均不包含 Key。
2. 密钥环不可用时不回退到明文文件。
3. `run` 不修改父进程环境，且只向子进程注入目标变量。
4. 环境副本不存在重复的旧 Key 变量。
5. 自定义前缀生成正确的变量名。
6. Stdin、Stdout、Stderr、信号和退出码正确传递。
7. 所有 Shell 导出脚本对特殊字符进行正确转义。
8. Agent 配置不包含明文、Base64 或拼接后的 Key。
9. Agent 配置写入失败时不破坏原文件。
10. 重命名 Profile 后 Key 仍可读取；删除后 Key 被清理。
11. JSON 输出无颜色、提示文本和敏感字段。
12. 非交互模式在缺少信息时安全失败，不挂起等待输入。
13. 错误、Verbose 和第三方库错误路径均不泄露 Key。
14. 并发写入不会导致 Profile 丢失或配置损坏。
15. Provider 预设 ID 唯一且顺序稳定；火山普通 API、Agent Plan、Coding Plan 的 Endpoint、Credential Type、Profile 和密钥环项相互隔离，旧配置缺省按 `api` 读取，`profile update` 不得修改类型。
16. 无参数 `akey` 与 `akey helper` 进入同一精简向导；`--help`、`--version` 行为不变，无参数 `--non-interactive` 安全失败。
17. Custom Base URL 在读取 Key 前校验；自动命名冲突按序号处理；Create → Set → Use 顺序正确，Key 保存失败时回滚新建 Profile。
18. Agent 0/1/N 检测、单选、跳过和确认语义正确；拒绝配置时零写入，`configured`、`unchanged`、`skipped`、`not_required` 和部分完成状态不得虚报。
19. 安装器通过脚本语法及真实安装测试；不支持的平台、下载失败和 SHA256 校验失败均安全停止并清理临时文件，PATH 更新幂等且不污染父 Shell。
20. GoReleaser snapshot 产出 9 个目标、对应压缩包和统一 SHA256；三平台安装 smoke test 验证安装路径、PATH 行为与 `akey --version`。

测试必须使用明显的伪密钥并在断言中检查其不出现；禁止在测试、快照、日志和 CI 环境中使用真实凭证。

## 10. 版本迭代规范

### 10.1 已定型，永久不动

- 命令树顶层结构及既有命令语义。
- 双存储安全架构。
- 子进程环境注入机制。
- 目录分层架构。
- 配置与密钥分离原则。
- Agent 适配器化原则。

确需不兼容变更时，必须升级主版本并提供迁移方案；不得静默改变已有命令的安全语义。

### 10.2 可扩展新增功能

- 用量查询。
- 模型列表。
- Agent 配置导入导出。
- DryRun 预检测。
- MCP 模板市场。
- 自动补全脚本。

扩展功能必须满足：

- 不读取或展示超出必要范围的密钥。
- 导入导出默认不包含 Key，Key 必须通过密钥环单独重新录入。
- DryRun 不得为了“模拟”而输出注入后的真实环境。
- 第三方 MCP 模板必须审查启动命令和网络行为，不得默认执行远程脚本。

## 11. 与 ArkCLI 的设计对齐总结

### 11.1 完全对齐

- Profile 环境体系。
- 安全密钥托管。
- `run` 子进程注入。
- Agent 一键配置体验。
- MCP 挂载逻辑。
- Helper 交互式向导。

### 11.2 差异化能力

- 适配全部已注册的本地 AI Agent，不限于单一模型平台。
- 架构更轻、体积更小。
- 完全开源可控。
- 可自定义环境变量前缀。
- 可部署 OpenWrt 和轻量服务器，但目标系统必须提供受支持的密钥环后端。

## 12. AI 开发强制约束（Trae 必看）

1. 绝对禁止密钥明文落地。
2. 绝对禁止污染宿主 Shell 环境。
3. 绝对禁止新增任何通过参数传递 Key 的 Flag。
4. 新增命令必须贴合本文档定义的 ArkCLI 语义，不得擅自变更既有命令树。
5. 所有 Agent 配置必须适配器化。
6. 核心逻辑与 UI 展示彻底解耦。
7. 所有错误必须优雅处理并返回稳定退出码。
8. 不得为了方便调试打印环境变量、密钥环值、完整请求头或隐藏输入。
9. 不得引入明文文件后端作为密钥环不可用时的降级方案。
10. 修改 Agent 配置前必须验证真实格式，保留用户已有内容，并支持失败恢复。
11. 新增或修改功能必须同步补充对应测试，尤其是凭证不泄露回归测试。
12. 本文档未明确允许的行为，不得在安全边界上自行推断或扩张。
