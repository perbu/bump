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
			expectCommit:  true, // commit happens even with no files
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
