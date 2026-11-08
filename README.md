# Akey-CLI

Akey-CLI（命令名 `akey`）是一个安全的 API Key 与本地 AI Agent 管理工具。它将 Profile 元数据与 API Key 严格分离，通过操作系统密钥环保存密钥，并仅在子进程的独立环境中注入所需变量。

支持 Trae、Claude Code、Codex，以及 MCP Server 的挂载、查询和移除。

## 核心特性

- **多 Profile 隔离**：分别管理 Provider、Base URL、Model 和环境变量前缀。
- **系统密钥环**：Windows 使用 WinCred，macOS 使用 Keychain，Linux 使用 Secret Service。
- **密钥零明文落盘**：不提供文件后端，不在密钥环不可用时降级保存明文。
- **纯净环境注入**：`akey run` 不经过 Shell，不修改父进程或当前终端环境。
- **Agent 适配器**：安全合并 Trae 和 Claude Code JSON 配置；通过官方 Codex CLI 管理 Codex MCP。
- **脚本与自动化**：支持表格和 JSON 输出、非交互模式及明确的退出码。
- **跨平台单二进制**：支持 Windows、macOS、Linux 和 OpenWrt 多架构构建。

## 一键安装（Go）

需要 Go 1.24 或更高版本：

```bash
go install github.com/Lensi-zhang/Akey-CLI/cmd/akey@v1.0.0
```

确认安装：

```bash
akey --version
```

`go install` 默认将二进制安装到 `go env GOPATH` 下的 `bin` 目录；请确保该目录已加入 `PATH`。

也可以从 [GitHub Releases](https://github.com/Lensi-zhang/Akey-CLI/releases/tag/v1.0.0) 下载对应平台的预编译二进制。

## 快速开始

### 1. 创建 Profile 并隐藏录入 Key

```bash
akey profile create volc-prod \
  --provider volcengine \
  --base-url https://ark.cn-beijing.volces.com/api/v3 \
  --model your-model-or-endpoint-id \
  --env-prefix API \
  --non-interactive

akey auth login --profile volc-prod
akey profile use volc-prod
```

API Key 只能通过隐藏输入录入，不提供 `--api-key` 或 `--key` 参数。

### 2. 在隔离环境中运行程序

```bash
akey run --profile volc-prod -- python app.py
```

默认注入以下变量：

```text
API_KEY
API_BASE_URL
API_MODEL
```

环境变量前缀可通过 Profile 的 `--env-prefix` 修改。`run` 会透传子进程的标准输入、输出、错误和退出码。

### 3. 配置 Agent 与 MCP

```bash
akey agent list
akey agent detect
akey agent configure trae --profile volc-prod

akey agent mcp add trae my-server \
  --profile volc-prod \
  --command your-mcp-server \
  --arg=--stdio

akey agent mcp list trae
akey agent mcp remove trae my-server --yes
```

Agent 中保存的启动形式为：

```text
akey run --profile <name> -- <server-command> [args...]
```

API Key 不会写入 Agent 配置；它在 MCP Server 启动时才从系统密钥环读取并注入子进程。

## 常用命令

```bash
akey helper                         # 分阶段交互式配置向导
akey profile list                   # 列出 Profile 和 Key 状态
akey profile show volc-prod         # 查看非敏感元数据
akey profile update volc-prod --model new-model
akey profile rename volc-prod volc-new
akey profile delete volc-new --yes

akey auth status --profile volc-prod
akey auth logout --profile volc-prod --yes

akey agent status
akey profile list --output json     # 机器可读输出
```

## 手动 Shell 会话（可选）

优先使用 `akey run`。确需在当前 Shell 中设置变量时，可以显式导出：

```bash
eval "$(akey env export --profile volc-prod --shell bash)"
eval "$(akey env unset --profile volc-prod --shell bash)"
```

`env export` 的输出包含真实 Key。导出后变量会存在于当前 Shell，并可能被子进程读取；使用完毕应执行 `env unset` 生成的脚本进行清理。为避免被结构化日志收集，`env export/unset` 不支持 JSON 输出。

## 平台要求

| 平台 | 密钥存储 | 构建目标 |
| --- | --- | --- |
| Windows | Windows 凭据管理器（WinCred） | AMD64、ARM64 |
| macOS | Keychain | AMD64、ARM64 |
| Linux | Secret Service | AMD64、ARM64、ARMv7 |
| OpenWrt | Secret Service 兼容后端 | MIPS、MIPSLE |

Linux 和 OpenWrt 需要可用的 Secret Service（例如 GNOME Keyring 及 D-Bus 会话）。密钥服务不可用时，相关命令会安全失败，不会写入明文文件。

## 从源码构建

```bash
git clone https://github.com/Lensi-zhang/Akey-CLI.git
cd Akey-CLI
go build -o akey ./cmd/akey
```

Windows 可将输出文件设为 `akey.exe`。运行测试：

```bash
go test ./...
go vet ./...
```

启用 CGO 和 C 编译器后，可执行竞态检测：

```bash
go test -race ./...
```

## 安全说明

- Profile 元数据保存在 `~/.akey/profiles.json`，不包含 API Key。
- 密钥以不可变 Profile ID 关联到系统密钥环，重命名 Profile 不会复制或重建密钥。
- 除用户显式调用 `env export` 外，任何命令都不会输出真实 Key。
- 不提供通过命令行参数传递 Key 的方式，避免 Shell 历史和进程列表泄露。
- Agent 配置只包含非敏感启动信息，不接收 Key。

详细设计和安全约束见 [正式开发文档](docs/DEVELOPMENT.md)。

## 问题反馈

请在 [GitHub Issues](https://github.com/Lensi-zhang/Akey-CLI/issues) 提交问题。不要附上真实 API Key、密钥环内容或包含凭据的 Shell 导出结果。
