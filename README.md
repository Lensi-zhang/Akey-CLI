# Akey-CLI

Akey-CLI（命令名 `akey`）是一个安全的 API Key 与本地 AI Agent 管理工具。它将 Profile 元数据与 API Key 严格分离，通过操作系统密钥环保存密钥，并仅在子进程的独立环境中注入所需变量。

支持火山方舟、OpenAI、DeepSeek、Moonshot AI、OpenRouter及自定义 OpenAI 兼容服务，并可初始化 Trae、Claude Code 和管理 Agent MCP。

## 核心特性

- **无需 Go 环境**：Windows、macOS、Linux 均可使用一条命令安装。
- **服务商预设**：自动填写 Base URL，普通 API Key、Agent Plan 和 Coding Plan 分开管理。
- **一次向导完成**：选择服务商、填写 Model、隐藏输入 Key；Profile 和默认环境自动处理。
- **系统密钥环**：Windows 使用 WinCred，macOS 使用 Keychain，Linux 使用 Secret Service。
- **密钥零明文落盘**：不提供文件后端，不在密钥环不可用时降级保存明文。
- **纯净环境注入**：`akey run` 不经过 Shell，不修改父进程或当前终端环境。
- **Agent 适配器**：安全合并 Trae 和 Claude Code JSON 配置；通过官方 Codex CLI 管理 Codex MCP。

## 一键安装

### Windows

在 PowerShell 中执行：

```powershell
irm https://github.com/Lensi-zhang/Akey-CLI/releases/latest/download/install.ps1 | iex
```

安装到 `%LOCALAPPDATA%\Programs\Akey\bin`，并加入当前用户 PATH。安装后重新打开终端。

### macOS / Linux

```bash
curl -fsSL https://github.com/Lensi-zhang/Akey-CLI/releases/latest/download/install.sh | sh
```

安装到 `$HOME/.local/bin`。如果安装器提示该目录不在 PATH，请先按提示将其加入 PATH，再重新打开终端。

也可以从 [GitHub Releases](https://github.com/Lensi-zhang/Akey-CLI/releases/latest) 手动下载对应平台的压缩包和 SHA256 校验文件。OpenWrt/MIPS 请使用手动安装。

## 快速开始

直接运行：

```bash
akey
```

按提示完成三步：

1. 选择服务商和 Key 类型。
2. 从服务商控制台复制并填写 Model 或 Endpoint ID。
3. 隐藏输入 API Key 或 Plan Key。

向导会自动创建 Profile、将 Key 保存到系统密钥环并设为当前默认环境。检测到 Trae 或 Claude Code 时，会在确认后初始化基础配置；该操作只准备 `mcpServers`，不会绑定模型或写入 Key。Codex 无需独立全局配置。

### 普通 Key 与 Plan Key 分开

火山方舟提供三个独立预设：

| 预设 | Key 类型 | Base URL |
| --- | --- | --- |
| 火山方舟（普通 API Key） | `api` | `https://ark.cn-beijing.volces.com/api/v3` |
| 火山方舟 Agent Plan | `agent-plan` | `https://ark.cn-beijing.volces.com/api/plan/v3` |
| 火山方舟 Coding Plan | `coding-plan` | `https://ark.cn-beijing.volces.com/api/coding/v3` |

每种类型使用不同 Profile、Endpoint 和密钥环项。即使录入的 Key 字符串相同，也不会自动共用凭证或切换计费通道。需要更换类型时请新建 Profile，不要在原 Profile 上混用。

其他预设包括 OpenAI、DeepSeek、Moonshot AI、OpenRouter 和 Custom。Custom 需要额外填写 Base URL。

### 安全运行程序

```bash
akey run --profile volcengine -- python app.py
```

默认注入 `API_KEY`、`API_BASE_URL`、`API_MODEL`，不修改当前终端环境。省略 `--profile` 时使用向导设置的默认 Profile。

## 高级用法

### Profile 与 Key

```bash
akey profile list
akey profile show volcengine
akey profile current
akey profile use volcengine

# 高级创建方式；Credential Type 创建后不可修改
akey profile create volc-plan \
  --provider volcengine \
  --credential-type agent-plan \
  --base-url https://ark.cn-beijing.volces.com/api/plan/v3 \
  --model your-model-or-endpoint-id \
  --non-interactive

# Key 只能通过隐藏输入录入，不支持 --api-key 或 --key
akey auth login --profile volc-plan
akey auth status --profile volc-plan
akey profile use volc-plan
```

支持的 `--credential-type` 为 `api`、`agent-plan`、`coding-plan`，缺省为 `api`。其他高级操作：

```bash
akey profile update volcengine --model new-model
akey profile rename volcengine volc-prod
akey profile delete volc-prod --yes
akey auth logout --profile volc-plan --yes
```

### Agent 与 MCP

```bash
akey agent list
akey agent detect
akey agent status
akey agent configure trae --profile volcengine

akey agent mcp add trae my-server \
  --profile volcengine \
  --command your-mcp-server \
  --arg=--stdio

akey agent mcp list trae
akey agent mcp remove trae my-server --yes
```

Agent 中保存的 MCP 启动形式为 `akey run --profile <name> -- <server-command> [args...]`。API Key 不会写入 Agent 配置，只在 MCP Server 启动时从系统密钥环读取并注入子进程。

`akey helper` 与无参数 `akey` 使用同一个精简向导。机器可读输出可使用 `--output json`，自动化可使用 `--non-interactive`。

### 手动 Shell 会话

优先使用 `akey run`。确需在当前 Shell 中设置变量时，可以显式导出：

```bash
eval "$(akey env export --profile volcengine --shell bash)"
eval "$(akey env unset --profile volcengine --shell bash)"
```

`env export` 的输出包含真实 Key。导出后变量会存在于当前 Shell，并可能被子进程读取；使用完毕应执行 `env unset` 生成的脚本进行清理。为避免被结构化日志收集，`env export/unset` 不支持 JSON 输出。

### 使用 Go 安装（开发者）

需要 Go 1.24 或更高版本：

```bash
go install github.com/Lensi-zhang/Akey-CLI/cmd/akey@v1.1.0
```

`go install` 默认安装到 `go env GOPATH` 下的 `bin` 目录，请确保该目录已加入 PATH。

### 从源码构建

```bash
git clone https://github.com/Lensi-zhang/Akey-CLI.git
cd Akey-CLI
go build -o akey ./cmd/akey
```

Windows 可将输出文件设为 `akey.exe`。运行测试：

```bash
go test ./...
go vet ./...
go test -race ./...
```

竞态检测需要启用 CGO 和可用的 C 编译器。

## 平台要求

| 平台 | 密钥存储 | 构建目标 |
| --- | --- | --- |
| Windows | Windows 凭据管理器（WinCred） | AMD64、ARM64 |
| macOS | Keychain | AMD64、ARM64 |
| Linux | Secret Service | AMD64、ARM64、ARMv7 |
| OpenWrt | Secret Service 兼容后端 | MIPS、MIPSLE |

Linux 和 OpenWrt 需要可用的 Secret Service（例如 GNOME Keyring 及 D-Bus 会话）。密钥服务不可用时，相关命令会安全失败，不会写入明文文件。

## 安全说明

- Profile 元数据保存在 `~/.akey/profiles.json`，不包含 API Key 或 Plan Key。
- 密钥以不可变 Profile ID 关联到系统密钥环；普通、Agent Plan、Coding Plan Profile 的密钥环项彼此独立。
- 除用户显式调用 `env export` 外，任何命令都不会输出真实 Key。
- 不提供通过命令行参数传递 Key 的方式，避免 Shell 历史和进程列表泄露。
- Agent 配置只包含非敏感启动信息，不接收 Key。

详细设计和安全约束见 [正式开发文档](docs/DEVELOPMENT.md)。

## 问题反馈

请在 [GitHub Issues](https://github.com/Lensi-zhang/Akey-CLI/issues) 提交问题。不要附上真实 API Key、Plan Key、密钥环内容或包含凭据的 Shell 导出结果。
