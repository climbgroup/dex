package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInstaller_ClaudeSettingsIntegration tests that a plugin with settings
// creates .claude/settings.json during installation.
// This is a full integration test that simulates real plugin installation.
func TestInstaller_ClaudeSettingsIntegration(t *testing.T) {
	// Create a temporary directory for the test project
	projectDir := t.TempDir()

	// Create a temporary directory for the test plugin
	pluginDir := t.TempDir()

	// Write package.hcl with settings
	packageHCL := `meta {
  name        = "test-settings-plugin"
  version     = "1.0.0"
  description = "Test plugin with settings"
  platforms   = ["claude-code"]
}

settings "mcp-permissions" {
  claude {
    allow = [
      "mcp__test-server",
      "Bash(docker:*)"
    ]
    deny = [
      "Bash(rm -rf /)"
    ]
    env = {
      TEST_VAR = "test_value"
    }
  }
}
`
	err := os.WriteFile(filepath.Join(pluginDir, "package.hcl"), []byte(packageHCL), 0644)
	require.NoError(t, err)

	// Write dex.hcl for the project
	projectHCL := `project {
  name = "test-project"
  default_platform = "claude-code"
}

package "test-settings-plugin" {
  source = "file://` + pluginDir + `"
}
`
	err = os.WriteFile(filepath.Join(projectDir, "dex.hcl"), []byte(projectHCL), 0644)
	require.NoError(t, err)

	// Create installer and run installation
	installer, err := NewInstaller(projectDir, "")
	require.NoError(t, err)

	_, err = installer.Install(nil)
	require.NoError(t, err)

	// Verify .claude directory was created
	claudeDir := filepath.Join(projectDir, ".claude")
	_, err = os.Stat(claudeDir)
	require.NoError(t, err, ".claude directory should exist")

	// Verify .claude/settings.json was created
	settingsPath := filepath.Join(claudeDir, "settings.json")
	_, err = os.Stat(settingsPath)
	require.NoError(t, err, ".claude/settings.json should exist")

	// Read and verify settings content
	settingsContent, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(settingsContent, &settings)
	require.NoError(t, err, "settings.json should be valid JSON")

	// Verify full settings content
	assert.Equal(t, map[string]any{
		"allow": []any{"mcp__test-server", "Bash(docker:*)"},
		"deny":  []any{"Bash(rm -rf /)"},
		"env":   map[string]any{"TEST_VAR": "test_value"},
	}, settings)
}

// TestInstaller_ClaudeSettingsWithOtherResources tests that settings
// works correctly when a plugin has multiple resource types.
func TestInstaller_ClaudeSettingsWithOtherResources(t *testing.T) {
	projectDir := t.TempDir()
	pluginDir := t.TempDir()

	// Create a skill file
	skillDir := filepath.Join(pluginDir, "skills")
	err := os.MkdirAll(skillDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(skillDir, "test.md"), []byte("Test skill content"), 0644)
	require.NoError(t, err)

	// Write package.hcl with multiple resources including settings
	packageHCL := `meta {
  name        = "multi-resource-plugin"
  version     = "1.0.0"
  description = "Plugin with multiple resource types"
  platforms   = ["claude-code"]
}

skill "test-skill" {
  description = "A test skill"
  content     = file("skills/test.md")
}

mcp_server "test-server" {
  command = "npx"
  args    = ["-y", "test-mcp"]
}

settings "permissions" {
  claude {
    allow = [
      "mcp__test-server"
    ]
  }
}
`
	err = os.WriteFile(filepath.Join(pluginDir, "package.hcl"), []byte(packageHCL), 0644)
	require.NoError(t, err)

	// Write dex.hcl
	projectHCL := `project {
  name = "test-project"
  default_platform = "claude-code"
}

package "multi-resource-plugin" {
  source = "file://` + pluginDir + `"
}
`
	err = os.WriteFile(filepath.Join(projectDir, "dex.hcl"), []byte(projectHCL), 0644)
	require.NoError(t, err)

	// Install
	installer, err := NewInstaller(projectDir, "")
	require.NoError(t, err)
	_, err = installer.Install(nil)
	require.NoError(t, err)

	// Verify skill was created
	skillPath := filepath.Join(projectDir, ".claude/skills/test-skill/SKILL.md")
	_, err = os.Stat(skillPath)
	assert.NoError(t, err, "skill should be created")

	// Verify MCP server was created
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	mcpContent, err := os.ReadFile(mcpPath)
	require.NoError(t, err)
	var mcpConfig map[string]any
	err = json.Unmarshal(mcpContent, &mcpConfig)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"mcpServers": map[string]any{
			"test-server": map[string]any{
				"command": "npx",
				"args":    []any{"-y", "test-mcp"},
			},
		},
	}, mcpConfig)

	// Verify settings file was created
	settingsPath := filepath.Join(projectDir, ".claude/settings.json")
	settingsContent, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	var settings map[string]any
	err = json.Unmarshal(settingsContent, &settings)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"allow": []any{"mcp__test-server"},
	}, settings)
}

// TestInstaller_ClaudeHookMergesIntoExistingSettings verifies a package that
// registers a Stop hook merges into a pre-existing .claude/settings.json without
// clobbering unrelated keys or hooks for other events, and that re-installing
// (a sync) does not duplicate the hook.
func TestInstaller_ClaudeHookMergesIntoExistingSettings(t *testing.T) {
	projectDir := t.TempDir()
	pluginDir := t.TempDir()

	packageHCL := `meta {
  name        = "hook-plugin"
  version     = "1.0.0"
  description = "Plugin that registers a Stop hook"
  platforms   = ["claude-code"]
}

settings "babysitter-hook" {
  claude {
    hook "Stop" {
      command = "python3 \"$CLAUDE_PROJECT_DIR/.claude/hooks/babysitter/babysitter.py\" hook"
    }
  }
}
`
	err := os.WriteFile(filepath.Join(pluginDir, "package.hcl"), []byte(packageHCL), 0644)
	require.NoError(t, err)

	projectHCL := `project {
  name = "test-project"
  default_platform = "claude-code"
}

package "hook-plugin" {
  source = "file://` + pluginDir + `"
}
`
	err = os.WriteFile(filepath.Join(projectDir, "dex.hcl"), []byte(projectHCL), 0644)
	require.NoError(t, err)

	// Pre-seed a settings.json with hand-written content dex must preserve:
	// an unrelated top-level key and a hook for a different event.
	claudeDir := filepath.Join(projectDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))
	settingsPath := filepath.Join(claudeDir, "settings.json")
	preexisting := map[string]any{
		"allow":      []any{"Read(*)"},
		"statusLine": map[string]any{"type": "command", "command": "my-status"},
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "hello.sh"}}},
			},
		},
	}
	seed, err := json.MarshalIndent(preexisting, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(settingsPath, seed, 0644))

	// Install twice to also assert idempotency (a re-sync must not duplicate).
	for i := 0; i < 2; i++ {
		installer, err := NewInstaller(projectDir, "")
		require.NoError(t, err)
		_, err = installer.Install(nil)
		require.NoError(t, err)
	}

	settingsContent, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	var settings map[string]any
	require.NoError(t, json.Unmarshal(settingsContent, &settings))

	// Unrelated keys survive.
	assert.Equal(t, []any{"Read(*)"}, settings["allow"])
	assert.Equal(t, map[string]any{"type": "command", "command": "my-status"}, settings["statusLine"])

	hooks, ok := settings["hooks"].(map[string]any)
	require.True(t, ok, "hooks should be present")

	// The pre-existing SessionStart hook is untouched.
	assert.Equal(t, []any{
		map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "hello.sh"}}},
	}, hooks["SessionStart"])

	// The Stop hook is registered exactly once despite two installs.
	stop, ok := hooks["Stop"].([]any)
	require.True(t, ok, "Stop hook should be present")
	require.Len(t, stop, 1, "Stop hook must not be duplicated across syncs")
	group := stop[0].(map[string]any)
	inner := group["hooks"].([]any)
	require.Len(t, inner, 1)
	cmd := inner[0].(map[string]any)
	assert.Equal(t, "command", cmd["type"])
	assert.Contains(t, cmd["command"], "babysitter.py")
}
