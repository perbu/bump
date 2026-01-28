package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestBumpWhenTagAlreadyExists(t *testing.T) {
	// Setup: Create temporary git repository
	tempDir, repo := setupTestRepo(t)
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	// Change to test repository directory
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create .version file with initial version
	versionFile := filepath.Join(tempDir, ".version")
	err = os.WriteFile(versionFile, []byte("v1.0.0"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Add and commit the .version file
	w, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add(".version")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("Add initial version file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create the v1.0.0 tag
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateTag("v1.0.0", head.Hash(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create v1.0.1 tag and also v1.0.2 tag to force a conflict
	// When bump tries to go from v1.0.1 -> v1.0.2, the tag will already exist
	_, err = repo.CreateTag("v1.0.1", head.Hash(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateTag("v1.0.2", head.Hash(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Count commits before running bump
	commitCountBefore := countCommits(t, repo)

	// Execute: Try to set a specific version that already exists
	// This should fail because v1.0.1 tag already exists
	var output bytes.Buffer
	err = run(context.Background(), &output, []string{"-version", "v1.0.1"}, nil)

	// Debug: Print what actually happened
	t.Logf("Output: %s", output.String())
	t.Logf("Error: %v", err)

	// Assertions: This demonstrates the bug
	// The bug is that even when tag creation fails, a commit was already made
	if err == nil {
		t.Error("Expected error when trying to create existing tag, but got none")
	}

	// Count commits after - there should be no new commits if properly handled
	commitCountAfter := countCommits(t, repo)
	if commitCountAfter != commitCountBefore {
		t.Errorf("BUG DETECTED: New commit was created even though tag creation should fail. Commit count changed from %d to %d",
			commitCountBefore, commitCountAfter)
	}

	// Verify .version file wasn't updated when tag creation fails
	content, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "v1.0.0" {
		t.Errorf("BUG DETECTED: .version file was updated even though tag creation should fail. Expected 'v1.0.0', got '%s'", string(content))
	}
}

func TestBumpNormalOperation(t *testing.T) {
	// Setup: Create temporary git repository
	tempDir, repo := setupTestRepo(t)
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	// Change to test repository directory
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create .version file with initial version
	versionFile := filepath.Join(tempDir, ".version")
	err = os.WriteFile(versionFile, []byte("v1.0.0"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Add and commit the .version file
	w, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add(".version")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("Add initial version file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create the v1.0.0 tag
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateTag("v1.0.0", head.Hash(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Make a change after the tag (simulating actual development work)
	changeFile := filepath.Join(tempDir, "feature.txt")
	err = os.WriteFile(changeFile, []byte("new feature"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add("feature.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("Add new feature", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Execute: Normal bump operation (v1.0.0 -> v1.0.1)
	var output bytes.Buffer
	err = run(context.Background(), &output, []string{"-patch"}, nil)

	// Should succeed
	if err != nil {
		t.Errorf("Expected no error for normal bump operation, but got: %v", err)
	}

	// Verify .version file was updated correctly
	content, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "v1.0.1" {
		t.Errorf("Expected .version file to be 'v1.0.1', but got '%s'", string(content))
	}

	// Verify the tag was created
	exists, err := tagExists(repo, "v1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("Expected tag v1.0.1 to be created, but it doesn't exist")
	}
}

// setupTestRepo creates a temporary git repository for testing
func setupTestRepo(t *testing.T) (string, *git.Repository) {
	tempDir := t.TempDir()

	// Initialize git repository
	repo, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create initial commit (git requires at least one commit for tags)
	w, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	// Create a dummy file for initial commit
	dummyFile := filepath.Join(tempDir, "README.md")
	err = os.WriteFile(dummyFile, []byte("# Test Repository"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = w.Add("README.md")
	if err != nil {
		t.Fatal(err)
	}

	_, err = w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	return tempDir, repo
}

// countCommits returns the number of commits in the repository
func countCommits(t *testing.T, repo *git.Repository) int {
	ref, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}

	cIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	err = cIter.ForEach(func(c *object.Commit) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return count
}

func TestLastTag(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		want    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "single tag",
			tags:    []string{"v1.0.0"},
			want:    "v1.0.0",
			wantErr: false,
		},
		{
			name:    "multiple tags in order",
			tags:    []string{"v1.0.0", "v1.0.1", "v1.0.2"},
			want:    "v1.0.2",
			wantErr: false,
		},
		{
			name:    "multiple tags out of order",
			tags:    []string{"v1.0.2", "v1.0.0", "v1.0.1"},
			want:    "v1.0.2",
			wantErr: false,
		},
		{
			name:    "mix of valid and invalid tags",
			tags:    []string{"v1.0.0", "not-a-version", "v1.0.1", "v2.0.0"},
			want:    "v2.0.0",
			wantErr: false,
		},
		{
			name:    "prerelease versions",
			tags:    []string{"v1.0.0", "v1.0.1-alpha", "v1.0.1"},
			want:    "v1.0.1",
			wantErr: false,
		},
		{
			name:    "major/minor/patch versions",
			tags:    []string{"v0.0.1", "v1.0.0", "v0.1.0"},
			want:    "v1.0.0",
			wantErr: false,
		},
		{
			name:    "no tags",
			tags:    []string{},
			want:    "",
			wantErr: true,
			errMsg:  "no version tags found in the repository",
		},
		{
			name:    "only invalid tags",
			tags:    []string{"not-a-version", "also-not-a-version"},
			want:    "",
			wantErr: true,
			errMsg:  "no version tags found in the repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary repository
			tempDir, repo := setupTestRepo(t)
			defer os.RemoveAll(tempDir)

			// Create the specified tags
			if len(tt.tags) > 0 {
				head, err := repo.Head()
				if err != nil {
					t.Fatal(err)
				}
				for _, tag := range tt.tags {
					_, err = repo.CreateTag(tag, head.Hash(), nil)
					if err != nil {
						t.Fatal(err)
					}
				}
			}

			// Call lastTag
			got, err := lastTag(repo)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("lastTag() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if err.Error() != tt.errMsg {
					t.Errorf("lastTag() error message = %v, want %v", err.Error(), tt.errMsg)
				}
			}

			// Check result
			if got != tt.want {
				t.Errorf("lastTag() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIncrementVersion(t *testing.T) {
	tests := []struct {
		name    string
		current string
		action  action
		want    string
		wantErr bool
	}{
		{
			name:    "increment patch",
			current: "v1.2.3",
			action:  incrementPatch,
			want:    "v1.2.4",
			wantErr: false,
		},
		{
			name:    "increment minor",
			current: "v1.2.3",
			action:  incrementMinor,
			want:    "v1.3.0",
			wantErr: false,
		},
		{
			name:    "increment major",
			current: "v1.2.3",
			action:  incrementMajor,
			want:    "v2.0.0",
			wantErr: false,
		},
		{
			name:    "increment patch from zero",
			current: "v0.0.0",
			action:  incrementPatch,
			want:    "v0.0.1",
			wantErr: false,
		},
		{
			name:    "increment minor resets patch",
			current: "v1.2.9",
			action:  incrementMinor,
			want:    "v1.3.0",
			wantErr: false,
		},
		{
			name:    "increment major resets minor and patch",
			current: "v1.9.9",
			action:  incrementMajor,
			want:    "v2.0.0",
			wantErr: false,
		},
		{
			name:    "large version numbers",
			current: "v999.999.999",
			action:  incrementPatch,
			want:    "v999.999.1000",
			wantErr: false,
		},
		{
			name:    "invalid action",
			current: "v1.0.0",
			action:  noAction,
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid version format - no dots",
			current: "v100",
			action:  incrementPatch,
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid version format - too many dots",
			current: "v1.0.0.0",
			action:  incrementPatch,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config{action: tt.action}
			got, err := incrementVersion(tt.current, cfg)

			if (err != nil) != tt.wantErr {
				t.Errorf("incrementVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("incrementVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBumpWithIgnoredFiles(t *testing.T) {
	// Setup: Create temporary git repository
	tempDir, repo := setupTestRepo(t)
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	// Change to test repository directory
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create .version file with initial version
	versionFile := filepath.Join(tempDir, ".version")
	err = os.WriteFile(versionFile, []byte("v1.0.0"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Add and commit the .version file
	w, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add(".version")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("Add initial version file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create the v1.0.0 tag
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateTag("v1.0.0", head.Hash(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create .gitignore to ignore .claude directory
	gitignoreFile := filepath.Join(tempDir, ".gitignore")
	err = os.WriteFile(gitignoreFile, []byte(".claude/\n.idea/\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add(".gitignore")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("Add .gitignore", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create ignored directory with a file (simulating .claude/settings.local.json)
	claudeDir := filepath.Join(tempDir, ".claude")
	err = os.MkdirAll(claudeDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	ignoredFile := filepath.Join(claudeDir, "settings.local.json")
	err = os.WriteFile(ignoredFile, []byte(`{"foo": "bar"}`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that git status shows clean (excluding ignored files)
	status, err := w.Status()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Git status: %+v", status)

	// Execute: Try to bump with ignored files present
	// This should succeed because ignored files shouldn't block bumping
	var output bytes.Buffer
	err = run(context.Background(), &output, []string{"-patch"}, nil)

	// Should succeed
	if err != nil {
		t.Errorf("Expected bump to succeed with ignored files present, but got error: %v", err)
		t.Logf("Output: %s", output.String())
	}

	// Verify .version file was updated correctly
	content, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "v1.0.1" {
		t.Errorf("Expected .version file to be 'v1.0.1', but got '%s'", string(content))
	}

	// Verify the tag was created
	exists, err := tagExists(repo, "v1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("Expected tag v1.0.1 to be created, but it doesn't exist")
	}
}

func TestBumpWithMetadataOnlyChanges(t *testing.T) {
	// This test simulates the issue seen in rabbitfs where go-git reports
	// a file as modified (worktree=77, staging=32) but git status shows clean.
	// This can happen with filemode changes, line endings, or other metadata differences.

	// Setup: Create temporary git repository
	tempDir, repo := setupTestRepo(t)
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	// Change to test repository directory
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create .version file with initial version
	versionFile := filepath.Join(tempDir, ".version")
	err = os.WriteFile(versionFile, []byte("v1.0.0"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a vendor file (simulating the rabbitfs scenario)
	vendorDir := filepath.Join(tempDir, "vendor", "github.com", "example", "pkg")
	err = os.MkdirAll(vendorDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	vendorFile := filepath.Join(vendorDir, "file.go")
	err = os.WriteFile(vendorFile, []byte("package pkg\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Add and commit both files
	w, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add(".version")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add(filepath.Join("vendor", "github.com", "example", "pkg", "file.go"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("Add initial files", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create the v1.0.0 tag
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateTag("v1.0.0", head.Hash(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Make an actual change after the tag
	changeFile := filepath.Join(tempDir, "feature.txt")
	err = os.WriteFile(changeFile, []byte("new feature"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add("feature.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("Add new feature", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Now rewrite the vendor file with identical content but potentially different metadata
	// This simulates the scenario where go-git thinks the file is modified
	// but git status shows clean (could be due to filemode, line endings, etc.)
	err = os.WriteFile(vendorFile, []byte("package pkg\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Check status - go-git might report this as modified
	status, err := w.Status()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Git status after rewrite: %+v", status)

	// Execute: Try to bump - this should succeed even if go-git reports the file as modified
	// because the content is identical to what's in git
	var output bytes.Buffer
	err = run(context.Background(), &output, []string{"-patch"}, nil)

	// Should succeed
	if err != nil {
		t.Errorf("Expected bump to succeed with metadata-only changes, but got error: %v", err)
		t.Logf("Output: %s", output.String())
	}

	// Verify .version file was updated correctly
	content, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "v1.0.1" {
		t.Errorf("Expected .version file to be 'v1.0.1', but got '%s'", string(content))
	}

	// Verify the tag was created
	exists, err := tagExists(repo, "v1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("Expected tag v1.0.1 to be created, but it doesn't exist")
	}
}

func TestLoadIgnoreRules(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []ignoreRule
		wantErr bool
	}{
		{
			name:    "empty file",
			content: "",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "comments and blank lines",
			content: "# This is a comment\n\n# Another comment\n",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "anchored pattern",
			content: "/vendor\n",
			want:    []ignoreRule{{pattern: "vendor", anchored: true}},
			wantErr: false,
		},
		{
			name:    "unanchored pattern",
			content: "testdata\n",
			want:    []ignoreRule{{pattern: "testdata", anchored: false}},
			wantErr: false,
		},
		{
			name:    "mixed patterns",
			content: "# Anchored patterns\n/vendor\n/node_modules\n\n# Unanchored patterns\ntestdata\n.cache\n",
			want: []ignoreRule{
				{pattern: "vendor", anchored: true},
				{pattern: "node_modules", anchored: true},
				{pattern: "testdata", anchored: false},
				{pattern: ".cache", anchored: false},
			},
			wantErr: false,
		},
		{
			name:    "whitespace trimming",
			content: "  /vendor  \n  testdata  \n",
			want: []ignoreRule{
				{pattern: "vendor", anchored: true},
				{pattern: "testdata", anchored: false},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tempDir := t.TempDir()
			ignoreFile := filepath.Join(tempDir, ".bumpignore")
			err := os.WriteFile(ignoreFile, []byte(tt.content), 0644)
			if err != nil {
				t.Fatal(err)
			}

			got, err := loadIgnoreRules(ignoreFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadIgnoreRules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("loadIgnoreRules() got %d rules, want %d", len(got), len(tt.want))
				return
			}

			for i, rule := range got {
				if rule.pattern != tt.want[i].pattern || rule.anchored != tt.want[i].anchored {
					t.Errorf("loadIgnoreRules() rule[%d] = %+v, want %+v", i, rule, tt.want[i])
				}
			}
		})
	}
}

func TestLoadIgnoreRulesNoFile(t *testing.T) {
	// Test that missing file returns nil, nil
	rules, err := loadIgnoreRules("/nonexistent/.bumpignore")
	if err != nil {
		t.Errorf("loadIgnoreRules() error = %v, want nil", err)
	}
	if rules != nil {
		t.Errorf("loadIgnoreRules() got %v, want nil", rules)
	}
}

func TestShouldIgnore(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		dirName string
		rules   []ignoreRule
		want    bool
	}{
		{
			name:    "no rules",
			path:    "vendor",
			dirName: "vendor",
			rules:   nil,
			want:    false,
		},
		{
			name:    "anchored match at root",
			path:    "vendor",
			dirName: "vendor",
			rules:   []ignoreRule{{pattern: "vendor", anchored: true}},
			want:    true,
		},
		{
			name:    "anchored no match nested",
			path:    "foo/vendor",
			dirName: "vendor",
			rules:   []ignoreRule{{pattern: "vendor", anchored: true}},
			want:    false,
		},
		{
			name:    "unanchored match at root",
			path:    "testdata",
			dirName: "testdata",
			rules:   []ignoreRule{{pattern: "testdata", anchored: false}},
			want:    true,
		},
		{
			name:    "unanchored match nested",
			path:    "foo/bar/testdata",
			dirName: "testdata",
			rules:   []ignoreRule{{pattern: "testdata", anchored: false}},
			want:    true,
		},
		{
			name:    "multiple rules first matches",
			path:    "vendor",
			dirName: "vendor",
			rules: []ignoreRule{
				{pattern: "vendor", anchored: true},
				{pattern: "testdata", anchored: false},
			},
			want: true,
		},
		{
			name:    "multiple rules second matches",
			path:    "foo/testdata",
			dirName: "testdata",
			rules: []ignoreRule{
				{pattern: "vendor", anchored: true},
				{pattern: "testdata", anchored: false},
			},
			want: true,
		},
		{
			name:    "no match",
			path:    "src",
			dirName: "src",
			rules: []ignoreRule{
				{pattern: "vendor", anchored: true},
				{pattern: "testdata", anchored: false},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldIgnore(tt.path, tt.dirName, tt.rules)
			if got != tt.want {
				t.Errorf("shouldIgnore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBumpWithBumpignore(t *testing.T) {
	// Setup: Create temporary git repository
	tempDir, repo := setupTestRepo(t)
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	// Change to test repository directory
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create .version file at root
	versionFile := filepath.Join(tempDir, ".version")
	err = os.WriteFile(versionFile, []byte("v1.0.0"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create .version file in a directory that should be ignored
	ignoredDir := filepath.Join(tempDir, "vendor", "pkg")
	err = os.MkdirAll(ignoredDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	ignoredVersionFile := filepath.Join(ignoredDir, ".version")
	err = os.WriteFile(ignoredVersionFile, []byte("v0.0.0"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create .version file in a nested directory that should be ignored (unanchored)
	nestedIgnoredDir := filepath.Join(tempDir, "foo", "testdata")
	err = os.MkdirAll(nestedIgnoredDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	nestedIgnoredVersionFile := filepath.Join(nestedIgnoredDir, ".version")
	err = os.WriteFile(nestedIgnoredVersionFile, []byte("v0.0.0"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create .version file in a directory that should NOT be ignored
	includedDir := filepath.Join(tempDir, "src")
	err = os.MkdirAll(includedDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	includedVersionFile := filepath.Join(includedDir, ".version")
	err = os.WriteFile(includedVersionFile, []byte("v1.0.0"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create .bumpignore file
	bumpignore := filepath.Join(tempDir, ".bumpignore")
	err = os.WriteFile(bumpignore, []byte("# Anchored patterns\n/vendor\n\n# Unanchored patterns\ntestdata\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Add and commit all files
	w, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add(".version")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add("src/.version")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add(".bumpignore")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("Add version files and .bumpignore", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create the v1.0.0 tag
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateTag("v1.0.0", head.Hash(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Make a change after the tag
	changeFile := filepath.Join(tempDir, "feature.txt")
	err = os.WriteFile(changeFile, []byte("new feature"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add("feature.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("Add new feature", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Execute: bump with .bumpignore
	var output bytes.Buffer
	err = run(context.Background(), &output, []string{"-patch"}, nil)

	// Should succeed
	if err != nil {
		t.Errorf("Expected bump to succeed, but got error: %v", err)
		t.Logf("Output: %s", output.String())
	}

	// Verify root .version was updated
	content, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "v1.0.1" {
		t.Errorf("Expected root .version to be 'v1.0.1', but got '%s'", string(content))
	}

	// Verify included .version was updated
	content, err = os.ReadFile(includedVersionFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "v1.0.1" {
		t.Errorf("Expected src/.version to be 'v1.0.1', but got '%s'", string(content))
	}

	// Verify ignored .version files were NOT updated
	content, err = os.ReadFile(ignoredVersionFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "v0.0.0" {
		t.Errorf("Expected vendor/pkg/.version to remain 'v0.0.0' (ignored), but got '%s'", string(content))
	}

	content, err = os.ReadFile(nestedIgnoredVersionFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "v0.0.0" {
		t.Errorf("Expected foo/testdata/.version to remain 'v0.0.0' (ignored), but got '%s'", string(content))
	}

	// Verify output mentions updated files but not ignored ones
	outputStr := output.String()
	if !strings.Contains(outputStr, ".version") {
		t.Errorf("Expected output to mention root .version update")
	}
	if !strings.Contains(outputStr, "src/.version") {
		t.Errorf("Expected output to mention src/.version update")
	}
	if strings.Contains(outputStr, "vendor") {
		t.Errorf("Expected output to NOT mention vendor (should be ignored)")
	}
	if strings.Contains(outputStr, "testdata") {
		t.Errorf("Expected output to NOT mention testdata (should be ignored)")
	}
}

func TestUpdateVersionFiles(t *testing.T) {
	tests := []struct {
		name          string
		versionFiles  map[string]string // path -> content
		newVersion    string
		dryRun        bool
		expectCommit  bool
		expectUpdated map[string]string // path -> expected content after
		wantErr       bool
		errContains   string
	}{
		{
			name: "single version file",
			versionFiles: map[string]string{
				".version": "v1.0.0",
			},
			newVersion:   "v1.0.1",
			dryRun:       false,
			expectCommit: true,
			expectUpdated: map[string]string{
				".version": "v1.0.1",
			},
			wantErr: false,
		},
		{
			name: "multiple version files",
			versionFiles: map[string]string{
				".version":     "v1.0.0",
				"foo/.version": "v1.0.0",
				"bar/.version": "v1.0.0",
			},
			newVersion:   "v2.0.0",
			dryRun:       false,
			expectCommit: true,
			expectUpdated: map[string]string{
				".version":     "v2.0.0",
				"foo/.version": "v2.0.0",
				"bar/.version": "v2.0.0",
			},
			wantErr: false,
		},
		{
			name: "dry run - no changes",
			versionFiles: map[string]string{
				".version": "v1.0.0",
			},
			newVersion:   "v1.0.1",
			dryRun:       true,
			expectCommit: false,
			expectUpdated: map[string]string{
				".version": "v1.0.0", // should remain unchanged
			},
			wantErr: false,
		},
		{
			name: "empty version file",
			versionFiles: map[string]string{
				".version": "",
			},
			newVersion:   "v1.0.0",
			dryRun:       false,
			expectCommit: true,
			expectUpdated: map[string]string{
				".version": "v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "invalid version in file",
			versionFiles: map[string]string{
				".version": "not-a-version",
			},
			newVersion:  "v1.0.0",
			dryRun:      false,
			wantErr:     true,
			errContains: "invalid version in file",
		},
		{
			name:          "no version files",
			versionFiles:  map[string]string{},
			newVersion:    "v1.0.0",
			dryRun:        false,
			expectCommit:  false, // no commit when no files are updated
			expectUpdated: map[string]string{},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary repository
			tempDir, repo := setupTestRepo(t)
			originalDir, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(originalDir)

			// Change to test repository directory
			err = os.Chdir(tempDir)
			if err != nil {
				t.Fatal(err)
			}

			// Create version files
			for path, content := range tt.versionFiles {
				dir := filepath.Dir(path)
				if dir != "." {
					err = os.MkdirAll(dir, 0755)
					if err != nil {
						t.Fatal(err)
					}
				}
				err = os.WriteFile(path, []byte(content), 0644)
				if err != nil {
					t.Fatal(err)
				}
			}

			// Count commits before
			commitsBefore := countCommits(t, repo)

			// Call updateVersionFiles
			var output bytes.Buffer
			cfg := config{dryRun: tt.dryRun}
			err = updateVersionFiles(repo, cfg, &output, tt.newVersion)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("updateVersionFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("updateVersionFiles() error = %v, want error containing %v", err, tt.errContains)
				}
			}

			// Check commits
			commitsAfter := countCommits(t, repo)
			if tt.expectCommit && commitsAfter != commitsBefore+1 {
				t.Errorf("Expected commit to be created, but commit count is %d (was %d)", commitsAfter, commitsBefore)
			}
			if !tt.expectCommit && commitsAfter != commitsBefore {
				t.Errorf("Expected no commit, but commit count changed from %d to %d", commitsBefore, commitsAfter)
			}

			// Check file contents
			for path, expectedContent := range tt.expectUpdated {
				content, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("Failed to read %s: %v", path, err)
					continue
				}
				if string(content) != expectedContent {
					t.Errorf("File %s: got content %q, want %q", path, string(content), expectedContent)
				}
			}

			// Check output contains update messages
			outputStr := output.String()
			if !tt.dryRun && !tt.wantErr {
				for path := range tt.versionFiles {
					expectedMsg := fmt.Sprintf("Updating version in file %s to %s", path, tt.newVersion)
					if !strings.Contains(outputStr, expectedMsg) {
						t.Errorf("Expected output to contain %q, but got: %s", expectedMsg, outputStr)
					}
				}
			}
		})
	}
}
