package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
