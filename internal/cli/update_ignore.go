// Package cli implements the command-line interface for dex.
package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"

	"github.com/climbgroup/dex/internal/manifest"
)

const (
	dexIgnoreStart = "# --- dex managed (do not edit) ---"
	dexIgnoreEnd   = "# --- end dex managed ---"
)

// updateIgnoreForProject updates the repository's single .git/info/exclude with
// the dex-managed files for the project at absPath. Called by sync when
// --git-exclude is set or git_exclude = true in dex.hcl. Also migrates any
// existing dex section from .gitignore to the new location.
//
// It works in a plain repo, a monorepo subdirectory, and a worktree: the real
// exclude file is located via `git rev-parse`, managed paths are written
// relative to the repo toplevel, and the managed block is keyed by the
// project's path within the repo so sibling projects in one repo never clobber
// each other's block.
func updateIgnoreForProject(absPath string) error {
	// Load manifest
	mf, err := manifest.Load(absPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	// Locate the repo's single info/exclude file and this project's offset from
	// the repo toplevel. Verifies absPath is inside a git repository.
	excludePath, relPrefix, err := resolveGitExclude(absPath)
	if err != nil {
		return err
	}

	// Build the dex section, prefixing every entry with the project's path so
	// the toplevel-relative exclude patterns resolve to the right files, and
	// keying the block markers on the same path.
	allFiles := mf.AllFiles()
	dexSection := buildDexIgnoreSection(relPrefix, allFiles)
	start, end := dexStart(relPrefix), dexEnd(relPrefix)

	// Ensure the info/ directory exists
	if err := os.MkdirAll(filepath.Dir(excludePath), 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(excludePath), err)
	}

	// Read existing info/exclude
	existingContent := ""
	if data, err := os.ReadFile(excludePath); err == nil {
		existingContent = string(data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read %s: %w", excludePath, err)
	}

	// Update only this project's dex section in exclude
	newContent := updateDexSection(existingContent, dexSection, start, end)

	// Write the updated info/exclude
	if err := os.WriteFile(excludePath, []byte(newContent), 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", excludePath, err)
	}

	// Migrate: remove old dex section from .gitignore if present.
	// Non-fatal: the core write to info/exclude already succeeded, so a failure
	// here (e.g. read-only .gitignore) leaves the section in both files temporarily
	// until the user resolves the permission issue.
	if err := migrateGitignore(absPath); err != nil {
		fmt.Printf("%s Warning: failed to migrate .gitignore: %v\n", color.YellowString("⚠"), err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	fmt.Printf("%s Updated %s with %d managed file(s)\n", green("✓"), excludePath, len(allFiles)+dexIgnoreAlwaysCount)

	return nil
}

// resolveGitExclude finds the repository's single info/exclude file and the
// project's path relative to the repo toplevel. Handles a plain repo (relPrefix
// ""), a monorepo subdirectory (relPrefix e.g. "apps/ntreecloud"), and
// worktrees or a .git file (via --git-common-dir). Returns a "not a git
// repository" error if absPath is not inside a work tree.
func resolveGitExclude(absPath string) (excludePath, relPrefix string, err error) {
	// --git-common-dir points at the shared git dir (the same one across all
	// worktrees), which is where info/exclude lives. It may be absolute or
	// relative to absPath; anchoring the relative form on absPath keeps us in
	// the caller's path namespace (avoids macOS /var vs /private/var symlink
	// mismatches that --show-toplevel would introduce).
	commonDir, err := gitOutput(absPath, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", "", fmt.Errorf("not a git repository: %s", absPath)
	}

	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(absPath, commonDir)
	}

	// --show-prefix gives the project's path relative to the repo toplevel,
	// forward-slashed with a trailing slash ("apps/ntreecloud/"), or empty at
	// the root. Asking git directly sidesteps symlink-resolution mismatches.
	prefix, err := gitOutput(absPath, "rev-parse", "--show-prefix")
	if err != nil {
		return "", "", fmt.Errorf("resolve repo prefix for %s: %w", absPath, err)
	}

	relPrefix = strings.TrimSuffix(prefix, "/")

	return filepath.Join(commonDir, "info", "exclude"), relPrefix, nil
}

// gitOutput runs git in dir and returns its trimmed stdout.
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

// dexStart returns the start marker for a project's managed block. The empty
// prefix (a single-project repo, or a project at the repo root) keeps the
// original unkeyed marker, so existing exclude files and standalone repos are
// unaffected.
func dexStart(relPrefix string) string {
	if relPrefix == "" {
		return dexIgnoreStart
	}

	return "# --- dex managed: " + relPrefix + " (do not edit) ---"
}

// dexEnd returns the end marker matching dexStart.
func dexEnd(relPrefix string) string {
	if relPrefix == "" {
		return dexIgnoreEnd
	}

	return "# --- end dex managed: " + relPrefix + " ---"
}

// migrateGitignore removes the dex managed section from .gitignore if present.
// This is called after writing to .git/info/exclude to clean up the old
// location. Legacy .gitignore sections always used the unkeyed markers.
func migrateGitignore(projectDir string) error {
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read .gitignore: %w", err)
	}

	content := string(data)
	if !strings.Contains(content, dexIgnoreStart) {
		return nil
	}

	newContent := stripDexSection(content, dexIgnoreStart, dexIgnoreEnd)
	if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write .gitignore: %w", err)
	}
	return nil
}

// stripDexSection removes the dex managed section bounded by start/end from
// content, if present. Returns the content unchanged if no section is found.
func stripDexSection(content, start, end string) string {
	startIdx := strings.Index(content, start)
	endIdx := strings.Index(content, end)
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return content
	}
	endIdx += len(end)
	// Skip trailing newline after end marker
	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}
	before := content[:startIdx]
	after := content[endIdx:]
	// Remove the extra blank line that preceded the section
	if strings.HasSuffix(before, "\n\n") {
		before = before[:len(before)-1]
	}
	return before + after
}

// dexIgnoreAlwaysCount is the number of entries buildDexIgnoreSection always writes
// regardless of manifest contents (currently: .dex/ and dex.lock).
// If buildDexIgnoreSection changes its always-included entries, update this constant
// and TestBuildDexIgnoreSection_AlwaysCount in the test file to match.
const dexIgnoreAlwaysCount = 2

// buildDexIgnoreSection builds the dex-managed section for info/exclude. Every
// entry is prefixed with relPrefix (the project's path within the repo) so the
// toplevel-relative exclude patterns resolve to the project's files; relPrefix
// "" leaves entries bare, matching a single-project repo.
func buildDexIgnoreSection(relPrefix string, allFiles []string) string {
	var dexSection strings.Builder
	dexSection.WriteString(dexStart(relPrefix))
	dexSection.WriteString("\n")

	// Always include .dex directory and lock file
	dexSection.WriteString(joinRel(relPrefix, ".dex/"))
	dexSection.WriteString("\n")
	dexSection.WriteString(joinRel(relPrefix, "dex.lock"))
	dexSection.WriteString("\n")

	for _, file := range allFiles {
		dexSection.WriteString(joinRel(relPrefix, file))
		dexSection.WriteString("\n")
	}

	dexSection.WriteString(dexEnd(relPrefix))
	dexSection.WriteString("\n")
	return dexSection.String()
}

// joinRel prefixes a project-relative managed path with the project's offset
// from the repo toplevel, using forward slashes (gitignore syntax). An empty
// prefix returns the path unchanged.
func joinRel(prefix, p string) string {
	if prefix == "" {
		return p
	}

	return prefix + "/" + p
}

// updateDexSection replaces or appends the dex section bounded by start/end in
// existingContent. Other projects' blocks (with different markers) are left
// untouched.
func updateDexSection(existingContent, dexSection, start, end string) string {
	// Check if there's an existing dex section for this project
	startIdx := strings.Index(existingContent, start)
	endIdx := strings.Index(existingContent, end)

	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		// Replace existing section
		endIdx += len(end)
		// Skip any trailing newline
		if endIdx < len(existingContent) && existingContent[endIdx] == '\n' {
			endIdx++
		}
		return existingContent[:startIdx] + dexSection + existingContent[endIdx:]
	}

	// Append new section
	if existingContent != "" && !strings.HasSuffix(existingContent, "\n") {
		existingContent += "\n"
	}
	if existingContent != "" {
		existingContent += "\n"
	}
	return existingContent + dexSection
}
