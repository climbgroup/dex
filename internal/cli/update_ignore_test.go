package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitInit creates a real git repository at dir and clears the default
// info/exclude template so tests can assert exact dex output.
func gitInit(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	require.NoError(t, cmd.Run(), "git init failed")

	// git init seeds .git/info/exclude with a comment template; clear it so the
	// dex section is the entire file and exact-equality assertions hold.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), nil, 0600))
}

// emptyManifest writes a minimal .dex/manifest.json under dir.
func emptyManifest(t *testing.T, dir string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".dex"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".dex", "manifest.json"),
		[]byte(`{"version":"1.0","plugins":{}}`),
		0644,
	))
}

func TestUpdateDexSection_ExactOutput_EmptyExisting(t *testing.T) {
	dexSection := dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		".claude/settings.json\n" +
		".claude/skills/a-skill.md\n" +
		".mcp.json\n" +
		"CLAUDE.md\n" +
		dexIgnoreEnd + "\n"

	result := updateDexSection("", dexSection, dexIgnoreStart, dexIgnoreEnd)

	expected := dexSection
	assert.Equal(t, expected, result)
}

func TestUpdateDexSection_ExactOutput_AppendsToExisting(t *testing.T) {
	existing := "node_modules/\n*.log\n"

	dexSection := dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		".mcp.json\n" +
		dexIgnoreEnd + "\n"

	result := updateDexSection(existing, dexSection, dexIgnoreStart, dexIgnoreEnd)

	expected := "node_modules/\n*.log\n\n" +
		dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		".mcp.json\n" +
		dexIgnoreEnd + "\n"
	assert.Equal(t, expected, result)
}

func TestUpdateDexSection_ReplacesExisting(t *testing.T) {
	existing := "node_modules/\n\n" +
		dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		".mcp.json\n" +
		dexIgnoreEnd + "\n" +
		"\n*.log\n"

	newDexSection := dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		".claude/settings.json\n" +
		".claude/skills/a-skill.md\n" +
		".mcp.json\n" +
		"CLAUDE.md\n" +
		dexIgnoreEnd + "\n"

	result := updateDexSection(existing, newDexSection, dexIgnoreStart, dexIgnoreEnd)

	expected := "node_modules/\n\n" +
		dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		".claude/settings.json\n" +
		".claude/skills/a-skill.md\n" +
		".mcp.json\n" +
		"CLAUDE.md\n" +
		dexIgnoreEnd + "\n" +
		"\n*.log\n"
	assert.Equal(t, expected, result)
}

func TestUpdateDexSection_ReplacesExisting_NothingAfter(t *testing.T) {
	existing := "node_modules/\n\n" +
		dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		dexIgnoreEnd + "\n"

	newDexSection := dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		".mcp.json\n" +
		"CLAUDE.md\n" +
		dexIgnoreEnd + "\n"

	result := updateDexSection(existing, newDexSection, dexIgnoreStart, dexIgnoreEnd)

	expected := "node_modules/\n\n" +
		dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		".mcp.json\n" +
		"CLAUDE.md\n" +
		dexIgnoreEnd + "\n"
	assert.Equal(t, expected, result)
}

// TestUpdateDexSection_TwoPathsCoexist verifies that two projects keyed by
// different paths write independent blocks into one exclude file: updating one
// must not disturb the other.
func TestUpdateDexSection_TwoPathsCoexist(t *testing.T) {
	node0Start, node0End := dexStart("apps/node0"), dexEnd("apps/node0")
	ntcStart, ntcEnd := dexStart("apps/ntreecloud"), dexEnd("apps/ntreecloud")

	node0Section := buildDexIgnoreSection("apps/node0", []string{".runbook/go-dev.yaml"})
	ntcSection := buildDexIgnoreSection("apps/ntreecloud", []string{".runbook/go-dev.yaml"})

	// Add node0, then ntreecloud.
	content := updateDexSection("", node0Section, node0Start, node0End)
	content = updateDexSection(content, ntcSection, ntcStart, ntcEnd)

	assert.Contains(t, content, "apps/node0/.runbook/go-dev.yaml")
	assert.Contains(t, content, "apps/ntreecloud/.runbook/go-dev.yaml")

	// Re-sync node0 with a changed file set; ntreecloud's block must survive verbatim.
	node0V2 := buildDexIgnoreSection("apps/node0", []string{".runbook/code-review.yaml"})
	content = updateDexSection(content, node0V2, node0Start, node0End)

	assert.Contains(t, content, "apps/node0/.runbook/code-review.yaml")
	assert.NotContains(t, content, "apps/node0/.runbook/go-dev.yaml")
	assert.Contains(t, content, ntcSection, "ntreecloud block must be untouched by a node0 re-sync")
}

func TestStripDexSection_WithSection(t *testing.T) {
	content := "node_modules/\n\n" +
		dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		dexIgnoreEnd + "\n" +
		"*.log\n"

	result := stripDexSection(content, dexIgnoreStart, dexIgnoreEnd)

	expected := "node_modules/\n*.log\n"
	assert.Equal(t, expected, result)
}

func TestStripDexSection_NoSection(t *testing.T) {
	content := "node_modules/\n*.log\n"
	result := stripDexSection(content, dexIgnoreStart, dexIgnoreEnd)
	assert.Equal(t, content, result)
}

func TestStripDexSection_InvertedMarkers(t *testing.T) {
	// End marker appears before start marker — malformed, must be a no-op.
	content := dexIgnoreEnd + "\n" + ".dex/\n" + dexIgnoreStart + "\n"
	result := stripDexSection(content, dexIgnoreStart, dexIgnoreEnd)
	assert.Equal(t, content, result)
}

func TestStripDexSection_SectionOnly(t *testing.T) {
	content := dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		dexIgnoreEnd + "\n"

	result := stripDexSection(content, dexIgnoreStart, dexIgnoreEnd)
	assert.Equal(t, "", result)
}

func TestMigrateGitignore_StripsOldSection(t *testing.T) {
	dir := t.TempDir()

	// Write a .gitignore with a dex managed section
	gitignoreContent := "node_modules/\n\n" +
		dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		dexIgnoreEnd + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignoreContent), 0644))

	err := migrateGitignore(dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "node_modules/\n", string(data))
}

func TestMigrateGitignore_NoOpWhenNoSection(t *testing.T) {
	dir := t.TempDir()

	original := "node_modules/\n*.log\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(original), 0644))

	err := migrateGitignore(dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, original, string(data))
}

func TestMigrateGitignore_NoOpWhenNoFile(t *testing.T) {
	dir := t.TempDir()
	err := migrateGitignore(dir)
	assert.NoError(t, err)
}

func TestUpdateIgnore_MigratesGitignore(t *testing.T) {
	dir := t.TempDir()

	emptyManifest(t, dir)
	gitInit(t, dir)

	// Pre-populate .gitignore with an old dex managed section.
	gitignoreContent := "node_modules/\n\n" +
		dexIgnoreStart + "\n" +
		".dex/\n" +
		"dex.lock\n" +
		dexIgnoreEnd + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignoreContent), 0644))

	err := updateIgnoreForProject(dir)
	require.NoError(t, err)

	// The dex section must have been removed from .gitignore.
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "node_modules/\n", string(data))

	// And written to .git/info/exclude instead.
	excludeData, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(excludeData), dexIgnoreStart),
		"exclude file should start with dex ignore marker")
}

func TestUpdateIgnore_ErrorsInNonGitDir(t *testing.T) {
	dir := t.TempDir()
	// Not inside a git work tree — should return an error, not create a repo.
	err := updateIgnoreForProject(dir)
	require.Error(t, err)
	assert.Equal(t, "not a git repository: "+dir, err.Error())

	// Confirm .git was NOT created.
	_, statErr := os.Stat(filepath.Join(dir, ".git"))
	assert.True(t, os.IsNotExist(statErr), ".git should not have been created")
}

func TestUpdateIgnore_WritesToGitInfoExclude(t *testing.T) {
	dir := t.TempDir()

	emptyManifest(t, dir)
	gitInit(t, dir)

	err := updateIgnoreForProject(dir)
	require.NoError(t, err)

	excludePath := filepath.Join(dir, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	require.NoError(t, err)

	// Project is at the repo root, so entries are unprefixed and the markers are
	// the unkeyed legacy markers — identical to dex's pre-monorepo output.
	expectedContent := dexIgnoreStart + "\n.dex/\ndex.lock\n" + dexIgnoreEnd + "\n"
	assert.Equal(t, expectedContent, string(data))
}

// TestUpdateIgnore_MonorepoSubdir is the end-to-end monorepo case: a project in
// a subdirectory writes a path-prefixed, path-keyed block into the repo-root
// info/exclude.
func TestUpdateIgnore_MonorepoSubdir(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)

	projDir := filepath.Join(root, "apps", "ntreecloud")
	require.NoError(t, os.MkdirAll(projDir, 0755))
	emptyManifest(t, projDir)

	err := updateIgnoreForProject(projDir)
	require.NoError(t, err)

	// No nested .git was created in the subdir.
	_, statErr := os.Stat(filepath.Join(projDir, ".git"))
	assert.True(t, os.IsNotExist(statErr), "subdir must not get its own .git")

	// The single repo-root exclude carries a path-keyed, path-prefixed block.
	data, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	require.NoError(t, err)

	expected := dexStart("apps/ntreecloud") + "\n" +
		"apps/ntreecloud/.dex/\n" +
		"apps/ntreecloud/dex.lock\n" +
		dexEnd("apps/ntreecloud") + "\n"
	assert.Equal(t, expected, string(data))
}

// TestResolveGitExclude_Subdir verifies the path resolution against real git.
func TestResolveGitExclude_Subdir(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)

	projDir := filepath.Join(root, "apps", "ntreecloud")
	require.NoError(t, os.MkdirAll(projDir, 0755))

	excludePath, relPrefix, err := resolveGitExclude(projDir)
	require.NoError(t, err)

	assert.Equal(t, "apps/ntreecloud", relPrefix)
	// Anchored on projDir, so it stays in the caller's path namespace.
	assert.Equal(t, filepath.Join(projDir, "..", "..", ".git", "info", "exclude"), excludePath)
}

// TestBuildDexIgnoreSection_MonorepoPrefix verifies entries and markers are
// keyed/prefixed by the project path.
func TestBuildDexIgnoreSection_MonorepoPrefix(t *testing.T) {
	section := buildDexIgnoreSection("apps/ntreecloud", []string{".runbook/go-dev.yaml", "CLAUDE.md"})

	expected := "# --- dex managed: apps/ntreecloud (do not edit) ---\n" +
		"apps/ntreecloud/.dex/\n" +
		"apps/ntreecloud/dex.lock\n" +
		"apps/ntreecloud/.runbook/go-dev.yaml\n" +
		"apps/ntreecloud/CLAUDE.md\n" +
		"# --- end dex managed: apps/ntreecloud ---\n"
	assert.Equal(t, expected, section)
}

// TestBuildDexIgnoreSection_AlwaysCount verifies that dexIgnoreAlwaysCount matches the
// number of entries buildDexIgnoreSection writes when allFiles is empty.
// If buildDexIgnoreSection changes its always-included entries, this test will fail,
// prompting an update to dexIgnoreAlwaysCount.
func TestBuildDexIgnoreSection_AlwaysCount(t *testing.T) {
	section := buildDexIgnoreSection("", nil)
	lines := 0
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && trimmed != dexIgnoreStart && trimmed != dexIgnoreEnd {
			lines++
		}
	}
	assert.Equal(t, dexIgnoreAlwaysCount, lines,
		"dexIgnoreAlwaysCount must equal the number of always-included entries in buildDexIgnoreSection")
}
