package resource

import "fmt"

// claudeHookEvents is the set of Claude Code lifecycle events a hook may target.
var claudeHookEvents = map[string]bool{
	"PreToolUse":       true,
	"PostToolUse":      true,
	"UserPromptSubmit": true,
	"Notification":     true,
	"Stop":             true,
	"SubagentStop":     true,
	"PreCompact":       true,
	"SessionStart":     true,
	"SessionEnd":       true,
}

// Settings represents a universal settings resource. Platform-specific settings
// go inside the platform override blocks (claude {}, copilot {}, cursor {}).
type Settings struct {
	Name      string   `hcl:"name,label"`
	Platforms []string `hcl:"platforms,optional"`

	Claude  *SettingsClaudeOverride `hcl:"claude,block"`
	Copilot *PlatformOverride       `hcl:"copilot,block"`
	Cursor  *PlatformOverride       `hcl:"cursor,block"`
}

// SettingsClaudeOverride contains all Claude-specific settings fields.
type SettingsClaudeOverride struct {
	Disabled                   bool              `hcl:"disabled,optional"`
	Allow                      []string          `hcl:"allow,optional"`
	Ask                        []string          `hcl:"ask,optional"`
	Deny                       []string          `hcl:"deny,optional"`
	Env                        map[string]string `hcl:"env,optional"`
	EnableAllProjectMCPServers bool              `hcl:"enable_all_project_mcp_servers,optional"`
	EnabledMCPServers          []string          `hcl:"enabled_mcp_servers,optional"`
	DisabledMCPServers         []string          `hcl:"disabled_mcp_servers,optional"`
	RespectGitignore           bool              `hcl:"respect_gitignore,optional"`
	IncludeCoAuthoredBy        bool              `hcl:"include_co_authored_by,optional"`
	Model                      string            `hcl:"model,optional"`
	OutputStyle                string            `hcl:"output_style,optional"`
	AlwaysThinkingEnabled      bool              `hcl:"always_thinking_enabled,optional"`
	PlansDirectory             string            `hcl:"plans_directory,optional"`
	AdditionalDirectories      []string          `hcl:"additional_directories,optional"`
	AutoMemoryDirectory        string            `hcl:"auto_memory_directory,optional"`
	IncludeGitInstructions     *bool             `hcl:"include_git_instructions,optional"`
	Agent                      string            `hcl:"agent,optional"`
	Hooks                      []ClaudeHookBlock `hcl:"hook,block"`
}

// ClaudeHookBlock declares a single Claude Code lifecycle hook to register in
// .claude/settings.json under the "hooks" key. The block label is the event
// name, e.g.
//
//	hook "Stop" {
//	  command = "python3 \"$CLAUDE_PROJECT_DIR/.claude/hooks/foo.py\""
//	}
//
//	hook "PreToolUse" {
//	  matcher = "Bash"
//	  command = "..."
//	  timeout = 30
//	}
//
// Hook commands may reference $CLAUDE_PROJECT_DIR (set by Claude Code) to locate
// scripts a package ships, so the path stays portable across machines.
type ClaudeHookBlock struct {
	// Event is the lifecycle event the hook fires on (the block label).
	Event string `hcl:"event,label"`

	// Matcher restricts the hook to matching tools/sources (event-dependent,
	// e.g. a tool name for PreToolUse). Empty means "all".
	Matcher string `hcl:"matcher,optional"`

	// Type is the hook handler type; defaults to "command".
	Type string `hcl:"type,optional"`

	// Command is the shell command to run.
	Command string `hcl:"command,attr"`

	// Timeout is an optional per-hook timeout in seconds (0 = unset).
	Timeout int `hcl:"timeout,optional"`
}

func (s *Settings) ResourceType() string                  { return "settings" }
func (s *Settings) ResourceName() string                  { return s.Name }
func (s *Settings) Platform() string                      { return "universal" }
func (s *Settings) GetContent() string                    { return "" }
func (s *Settings) GetFiles() []FileBlock                 { return nil }
func (s *Settings) GetTemplateFiles() []TemplateFileBlock { return nil }

func (s *Settings) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("settings: name is required")
	}
	if s.Claude != nil {
		for _, h := range s.Claude.Hooks {
			if h.Command == "" {
				return fmt.Errorf("settings %q: hook %q requires a command", s.Name, h.Event)
			}
			if !claudeHookEvents[h.Event] {
				return fmt.Errorf("settings %q: unknown hook event %q", s.Name, h.Event)
			}
			if h.Type != "" && h.Type != "command" {
				return fmt.Errorf("settings %q: hook %q has unsupported type %q (only \"command\")", s.Name, h.Event, h.Type)
			}
		}
	}
	return nil
}

func (s *Settings) IsEnabledForPlatform(platform string) bool {
	if len(s.Platforms) == 0 {
		return true
	}
	for _, p := range s.Platforms {
		if p == platform {
			return true
		}
	}
	return false
}
