package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const AkeyCommand = "akey"

var (
	namePattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	ErrInvalidServer = errors.New("invalid MCP server")
	ErrNotManaged    = errors.New("MCP server is not managed by akey")
)

// Server 描述由 akey 启动的服务进程，不保存任何凭证。
// Profile 仅在进程启动时由 akey run 解析。
type Server struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Profile string   `json:"profile"`
}

// Entry 是向 MCP 客户端提供的非敏感 stdio 配置。
// env、headers 等未知输入字段不会进入该结构。
type Entry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Type    string   `json:"type,omitempty"`
}

// Document 是标准的顶层 mcpServers JSON 结构。
type Document struct {
	MCPServers map[string]Entry `json:"mcpServers"`
}

// Template 仅包含占位可执行命令，不代表第三方 MCP 服务已经安装。
type Template struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

var builtinTemplates = []Template{
	{Name: "web-search", Command: "web-search-mcp", Args: []string{}},
	{Name: "code-execute", Command: "code-execute-mcp", Args: []string{}},
	{Name: "memory", Command: "memory-mcp", Args: []string{}},
}

// Templates 返回受控非敏感占位模板的副本。
// 调用方必须自行提供或安装实际的可执行程序。
func Templates() []Template {
	result := make([]Template, len(builtinTemplates))
	for i, template := range builtinTemplates {
		result[i] = template
		result[i].Args = append([]string(nil), template.Args...)
		if result[i].Args == nil {
			result[i].Args = []string{}
		}
	}
	return result
}

// Builtin 根据受控占位模板构造服务。
func Builtin(name, profile string) (Server, error) {
	for _, template := range builtinTemplates {
		if template.Name == name {
			return Custom(name, template.Command, template.Args, profile)
		}
	}
	return Server{}, fmt.Errorf("%w: unknown built-in template %q", ErrInvalidServer, name)
}

// Custom 构造并校验自定义 MCP 服务，不调用 Shell。
func Custom(name, command string, args []string, profile string) (Server, error) {
	server := Server{
		Name:    name,
		Command: command,
		Args:    append([]string(nil), args...),
		Profile: profile,
	}
	if server.Args == nil {
		server.Args = []string{}
	}
	if err := server.Validate(); err != nil {
		return Server{}, err
	}
	return server, nil
}

// ValidateName 应用 Profile 与 MCP 服务的名称规则。
func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("%w: name must match %s", ErrInvalidServer, namePattern.String())
	}
	return nil
}

// Validate 仅校验启动元数据，绝不访问凭证。
func (s Server) Validate() error {
	if err := ValidateName(s.Name); err != nil {
		return err
	}
	if err := ValidateName(s.Profile); err != nil {
		return fmt.Errorf("%w: invalid profile", err)
	}
	if strings.TrimSpace(s.Command) == "" {
		return fmt.Errorf("%w: command is required", ErrInvalidServer)
	}
	if strings.IndexByte(s.Command, 0) >= 0 {
		return fmt.Errorf("%w: command contains a NUL byte", ErrInvalidServer)
	}
	for _, arg := range s.Args {
		if strings.IndexByte(arg, 0) >= 0 {
			return fmt.Errorf("%w: argument contains a NUL byte", ErrInvalidServer)
		}
	}
	return nil
}

// Entry 将服务元数据转换为规定的 akey run 启动形式。
func (s Server) Entry() (Entry, error) {
	if err := s.Validate(); err != nil {
		return Entry{}, err
	}
	args := []string{"run", "--profile", s.Profile, "--", s.Command}
	args = append(args, s.Args...)
	return Entry{Command: AkeyCommand, Args: args}, nil
}

// EmptyDocument 返回将 mcpServers 序列化为对象的空文档。
func EmptyDocument() Document {
	return Document{MCPServers: make(map[string]Entry)}
}

// ToDocument 将已校验的服务元数据转换为标准 MCP 文档。
func ToDocument(servers []Server) (Document, error) {
	document := EmptyDocument()
	for _, server := range servers {
		if _, exists := document.MCPServers[server.Name]; exists {
			return Document{}, fmt.Errorf("%w: duplicate server %q", ErrInvalidServer, server.Name)
		}
		entry, err := server.Entry()
		if err != nil {
			return Document{}, err
		}
		document.MCPServers[server.Name] = entry
	}
	return document, nil
}

// JSON 将服务转换为不含凭证的缩进 mcpServers JSON。
func JSON(servers []Server) ([]byte, error) {
	document, err := ToDocument(servers)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(document, "", "  ")
}

// ManagedProfile 识别严格的 akey stdio 启动形式。
// 其他命令及格式异常的 akey 参数均视为外部配置。
func ManagedProfile(entry Entry) (string, bool) {
	if entry.Command != AkeyCommand || (entry.Type != "" && entry.Type != "stdio") || len(entry.Args) < 5 {
		return "", false
	}
	args := entry.Args
	if args[0] != "run" || args[1] != "--profile" || args[3] != "--" {
		return "", false
	}
	if ValidateName(args[2]) != nil || strings.TrimSpace(args[4]) == "" {
		return "", false
	}
	for _, arg := range args[4:] {
		if strings.IndexByte(arg, 0) >= 0 {
			return "", false
		}
	}
	return args[2], true
}

// IsManaged 报告条目是否为 akey 管理的 stdio 启动配置。
func IsManaged(entry Entry) bool {
	_, ok := ManagedProfile(entry)
	return ok
}

// ServerFromEntry 将 akey 管理的条目还原为启动元数据。
func ServerFromEntry(name string, entry Entry) (Server, error) {
	profile, ok := ManagedProfile(entry)
	if !ok {
		return Server{}, fmt.Errorf("%w: %q", ErrNotManaged, name)
	}
	return Custom(name, entry.Args[4], entry.Args[5:], profile)
}
