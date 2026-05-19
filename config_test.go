package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestLoadBumpConfig(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantNil   bool
		wantFiles int
		wantErr   bool
		errSub    string
	}{
		{
			name:    "missing file returns nil",
			path:    "testdata/configs/does-not-exist.toml",
			wantNil: true,
		},
		{
			name:      "cargo preset",
			path:      "testdata/configs/cargo-preset.toml",
			wantFiles: 1,
		},
		{
			name:      "npm preset",
			path:      "testdata/configs/npm-preset.toml",
			wantFiles: 1,
		},
		{
			name:      "chart preset",
			path:      "testdata/configs/chart-preset.toml",
			wantFiles: 1,
		},
		{
			name:      "custom regex",
			path:      "testdata/configs/custom-regex.toml",
			wantFiles: 1,
		},
		{
			name:      "multiple files",
			path:      "testdata/configs/multiple-files.toml",
			wantFiles: 3,
		},
		{
			name:    "invalid toml",
			path:    "testdata/configs/invalid.toml",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadBumpConfig(tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadBumpConfig() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("err %q does not contain %q", err.Error(), tt.errSub)
				}
				return
			}
			if tt.wantNil {
				if cfg != nil {
					t.Errorf("expected nil config, got %+v", cfg)
				}
				return
			}
			if cfg == nil {
				t.Fatalf("expected non-nil config")
			}
			if len(cfg.Files) != tt.wantFiles {
				t.Errorf("got %d files, want %d", len(cfg.Files), tt.wantFiles)
			}
		})
	}
}

func TestResolveRules(t *testing.T) {
	tests := []struct {
		name      string
		entry     fileEntry
		wantRules int
		wantErr   bool
		errSub    string
	}{
		{
			name:      "cargo preset",
			entry:     fileEntry{Path: "Cargo.toml", Format: "cargo"},
			wantRules: 1,
		},
		{
			name:      "npm preset",
			entry:     fileEntry{Path: "package.json", Format: "npm"},
			wantRules: 1,
		},
		{
			name:      "chart preset",
			entry:     fileEntry{Path: "Chart.yaml", Format: "chart"},
			wantRules: 2,
		},
		{
			name: "custom replace rules",
			entry: fileEntry{
				Path: "x.txt",
				Replace: []replaceRule{
					{Match: "a", Template: "b"},
				},
			},
			wantRules: 1,
		},
		{
			name:    "unknown preset",
			entry:   fileEntry{Path: "x", Format: "java"},
			wantErr: true,
			errSub:  "unknown format",
		},
		{
			name: "both format and replace",
			entry: fileEntry{
				Path:    "x",
				Format:  "cargo",
				Replace: []replaceRule{{Match: "a", Template: "b"}},
			},
			wantErr: true,
			errSub:  "cannot set both",
		},
		{
			name:    "neither format nor replace",
			entry:   fileEntry{Path: "x"},
			wantErr: true,
			errSub:  "must set",
		},
		{
			name:    "missing path",
			entry:   fileEntry{Format: "cargo"},
			wantErr: true,
			errSub:  "path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := resolveRules(tt.entry)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveRules() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("err %q does not contain %q", err.Error(), tt.errSub)
				}
				return
			}
			if len(rules) != tt.wantRules {
				t.Errorf("got %d rules, want %d", len(rules), tt.wantRules)
			}
		})
	}
}

func TestExpandTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		version  string
		want     string
	}{
		{name: "bare version", template: "v={version}", version: "1.2.3", want: "v=1.2.3"},
		{name: "bare version strips v prefix", template: "{version}", version: "v1.2.3", want: "1.2.3"},
		{name: "tag adds v prefix", template: "{tag}", version: "1.2.3", want: "v1.2.3"},
		{name: "tag keeps v prefix", template: "{tag}", version: "v1.2.3", want: "v1.2.3"},
		{name: "major", template: "{major}", version: "1.2.3", want: "1"},
		{name: "minor", template: "{minor}", version: "v1.2.3", want: "2"},
		{name: "patch", template: "{patch}", version: "1.2.3", want: "3"},
		{name: "combined", template: "{major}.{minor}", version: "1.2.3", want: "1.2"},
		{name: "preserves regex backrefs", template: "${1}{version}", version: "1.2.3", want: "${1}1.2.3"},
		{name: "no variables", template: "literal", version: "1.2.3", want: "literal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandTemplate(tt.template, tt.version)
			if got != tt.want {
				t.Errorf("expandTemplate(%q, %q) = %q, want %q", tt.template, tt.version, got, tt.want)
			}
		})
	}
}

func TestDetectVPrefix(t *testing.T) {
	tests := []struct {
		name  string
		match string
		want  bool
	}{
		{name: "bare cargo", match: `version = "1.0.0"`, want: false},
		{name: "v-prefixed cargo", match: `version = "v1.0.0"`, want: true},
		{name: "bare yaml", match: `version: 0.5.0`, want: false},
		{name: "v-prefixed yaml", match: `version: v0.5.0`, want: true},
		{name: "bare with prerelease", match: `version = "1.0.0-rc.1"`, want: false},
		{name: "v-prefixed in quotes", match: `appVersion: "v1.2.3"`, want: true},
		{name: "v as part of other word does not count", match: `apiVersion: v2`, want: false},
		{name: "no version at all", match: `name: myapp`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectVPrefix([]byte(tt.match))
			if got != tt.want {
				t.Errorf("detectVPrefix(%q) = %v, want %v", tt.match, got, tt.want)
			}
		})
	}
}

func TestApplyReplacementsAuto(t *testing.T) {
	tests := []struct {
		name    string
		content string
		rules   []replaceRule
		version string
		want    string
	}{
		{
			name:    "auto preserves bare prefix",
			content: `version = "1.0.0"` + "\n",
			rules: []replaceRule{
				{Match: `^version\s*=\s*"[^"]*"`, Template: `version = "{auto}"`},
			},
			version: "1.2.3",
			want:    `version = "1.2.3"` + "\n",
		},
		{
			name:    "auto preserves v prefix",
			content: `version = "v1.0.0"` + "\n",
			rules: []replaceRule{
				{Match: `^version\s*=\s*"[^"]*"`, Template: `version = "{auto}"`},
			},
			version: "1.2.3",
			want:    `version = "v1.2.3"` + "\n",
		},
		{
			name:    "auto with v-prefixed input version still respects per-match",
			content: `version = "1.0.0"` + "\n",
			rules: []replaceRule{
				{Match: `^version\s*=\s*"[^"]*"`, Template: `version = "{auto}"`},
			},
			version: "v1.2.3",
			want:    `version = "1.2.3"` + "\n",
		},
		{
			name:    "mixed prefix in same file resolves per match",
			content: "version: 0.5.0\nappVersion: \"v1.0.0\"\n",
			rules: []replaceRule{
				{Match: `^version:\s*.*$`, Template: `version: {auto}`},
				{Match: `^appVersion:\s*.*$`, Template: `appVersion: "{auto}"`},
			},
			version: "1.2.3",
			want:    "version: 1.2.3\nappVersion: \"v1.2.3\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyReplacements([]byte(tt.content), tt.rules, tt.version)
			if err != nil {
				t.Fatalf("applyReplacements: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", string(got), tt.want)
			}
		})
	}
}

func TestApplyReplacements(t *testing.T) {
	tests := []struct {
		name    string
		content string
		rules   []replaceRule
		version string
		want    string
		wantErr bool
		errSub  string
	}{
		{
			name:    "single match replace all",
			content: "version: 1.0.0\n",
			rules: []replaceRule{
				{Match: `(?m)^version:.*$`, Template: "version: {version}"},
			},
			version: "1.2.3",
			want:    "version: 1.2.3\n",
		},
		{
			name:    "multiple matches all replaced",
			content: "version: 1.0.0\nother\nversion: 1.0.0\n",
			rules: []replaceRule{
				{Match: `(?m)^version:.*$`, Template: "version: {version}"},
			},
			version: "1.2.3",
			want:    "version: 1.2.3\nother\nversion: 1.2.3\n",
		},
		{
			name:    "zero matches errors",
			content: "no version here\n",
			rules: []replaceRule{
				{Match: `(?m)^version:.*$`, Template: "version: {version}"},
			},
			version: "1.2.3",
			wantErr: true,
			errSub:  "no match",
		},
		{
			name:    "multiple rules all must match",
			content: "version: 0.0.0\nappVersion: 0.0.0\n",
			rules: []replaceRule{
				{Match: `(?m)^version:.*$`, Template: "version: {version}"},
				{Match: `(?m)^appVersion:.*$`, Template: `appVersion: "{version}"`},
			},
			version: "1.2.3",
			want:    "version: 1.2.3\nappVersion: \"1.2.3\"\n",
		},
		{
			name:    "second rule no match errors",
			content: "version: 0.0.0\n",
			rules: []replaceRule{
				{Match: `(?m)^version:.*$`, Template: "version: {version}"},
				{Match: `(?m)^appVersion:.*$`, Template: `appVersion: "{version}"`},
			},
			version: "1.2.3",
			wantErr: true,
			errSub:  "no match",
		},
		{
			name:    "tag template",
			content: "image: foo:v0.1.0\n",
			rules: []replaceRule{
				{Match: `(?m)^image: foo:.*$`, Template: "image: foo:{tag}"},
			},
			version: "1.2.3",
			want:    "image: foo:v1.2.3\n",
		},
		{
			name:    "invalid regex errors",
			content: "x\n",
			rules: []replaceRule{
				{Match: `[invalid`, Template: "x"},
			},
			version: "1.2.3",
			wantErr: true,
			errSub:  "compile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyReplacements([]byte(tt.content), tt.rules, tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("applyReplacements() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("err %q does not contain %q", err.Error(), tt.errSub)
				}
				return
			}
			if string(got) != tt.want {
				t.Errorf("applyReplacements() =\n%q\nwant\n%q", string(got), tt.want)
			}
		})
	}
}

func TestApplyReplacementsPresets(t *testing.T) {
	// Use the input/expected pairs in testdata/<case>/ to verify presets transform
	// real-world files correctly, including v-prefix preservation.
	cases := []struct {
		dir    string
		format string
		input  string
		want   string
	}{
		{dir: "cargo", format: "cargo", input: "Cargo.toml", want: "Cargo.toml.want"},
		{dir: "cargo-v", format: "cargo", input: "Cargo.toml", want: "Cargo.toml.want"},
		{dir: "npm", format: "npm", input: "package.json", want: "package.json.want"},
		{dir: "npm-v", format: "npm", input: "package.json", want: "package.json.want"},
		{dir: "chart", format: "chart", input: "Chart.yaml", want: "Chart.yaml.want"},
		{dir: "chart-v", format: "chart", input: "Chart.yaml", want: "Chart.yaml.want"},
		{dir: "mixed-prefix", format: "chart", input: "Chart.yaml", want: "Chart.yaml.want"},
	}

	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("testdata", tc.dir, tc.input))
			if err != nil {
				t.Fatal(err)
			}
			wantContent, err := os.ReadFile(filepath.Join("testdata", tc.dir, tc.want))
			if err != nil {
				t.Fatal(err)
			}

			rules, err := resolveRules(fileEntry{Path: tc.input, Format: tc.format})
			if err != nil {
				t.Fatal(err)
			}

			got, err := applyReplacements(content, rules, "1.2.3")
			if err != nil {
				t.Fatalf("applyReplacements: %v", err)
			}
			if !bytes.Equal(got, wantContent) {
				t.Errorf("preset %s mismatch.\nGOT:\n%s\nWANT:\n%s", tc.format, got, wantContent)
			}
		})
	}
}

// copyDir copies files from src to dst, skipping any file whose name ends with ".want".
// Used by integration tests to stage testdata into a temp repo.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if strings.HasSuffix(d.Name(), ".want") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// prepareBumpRepo wires up the shared scaffold for integration tests: temp
// repo, chdir, run setup(tempDir) to stage fixtures, commit + tag
// initialVersion, then add a feature commit so the bump has changes to act on.
// The chdir is restored via t.Cleanup.
func prepareBumpRepo(t *testing.T, initialVersion string, setup func(tempDir string)) (string, *git.Repository) {
	t.Helper()
	tempDir, repo := setupTestRepo(t)
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	if setup != nil {
		setup(tempDir)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".version"), []byte(initialVersion), 0644); err != nil {
		t.Fatal(err)
	}
	stageAndCommit(t, repo, "initial")
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTag(initialVersion, head.Hash(), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	stageAndCommit(t, repo, "feature")
	return tempDir, repo
}

// stageAndCommit adds every non-".want" file under root to the worktree and
// commits, returning the resulting HEAD.
func stageAndCommit(t *testing.T, repo *git.Repository, message string) {
	t.Helper()
	w, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	root := w.Filesystem.Root()
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, err = w.Add(rel)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "t@e.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBumpWithBumpConfig(t *testing.T) {
	cases := []struct {
		name      string
		dir       string
		checkPath []string // files to check against .want counterparts
	}{
		{name: "cargo", dir: "cargo", checkPath: []string{"Cargo.toml"}},
		{name: "cargo v-prefix preserved", dir: "cargo-v", checkPath: []string{"Cargo.toml"}},
		{name: "npm", dir: "npm", checkPath: []string{"package.json"}},
		{name: "npm v-prefix preserved", dir: "npm-v", checkPath: []string{"package.json"}},
		{name: "chart", dir: "chart", checkPath: []string{"Chart.yaml"}},
		{name: "chart v-prefix preserved", dir: "chart-v", checkPath: []string{"Chart.yaml"}},
		{name: "mixed prefix per line", dir: "mixed-prefix", checkPath: []string{"Chart.yaml"}},
		{name: "custom regex", dir: "custom", checkPath: []string{"release.yaml"}},
		{name: "multiple files", dir: "multi", checkPath: []string{
			"Cargo.toml", "package.json", "charts/foo/Chart.yaml",
		}},
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srcDir := filepath.Join(originalDir, "testdata", tc.dir)
			tempDir, _ := prepareBumpRepo(t, "v1.2.2", func(tempDir string) {
				copyDir(t, srcDir, tempDir)
			})

			var output bytes.Buffer
			err := run(context.Background(), &output, []string{"-patch"}, nil)
			if err != nil {
				t.Fatalf("run() err = %v\noutput:\n%s", err, output.String())
			}

			for _, p := range tc.checkPath {
				got, err := os.ReadFile(filepath.Join(tempDir, p))
				if err != nil {
					t.Fatalf("read %s: %v", p, err)
				}
				want, err := os.ReadFile(filepath.Join(srcDir, p+".want"))
				if err != nil {
					t.Fatalf("read want %s: %v", p, err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%s mismatch.\nGOT:\n%s\nWANT:\n%s", p, got, want)
				}
			}
		})
	}
}

// A .bump.toml referencing a file that the pattern can't match must return an
// error and leave no new commit and no new tag (atomic abort).
func TestBumpConfigNoMatchAborts(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(originalDir, "testdata", "no-match")
	_, repo := prepareBumpRepo(t, "v1.0.0", func(tempDir string) {
		copyDir(t, srcDir, tempDir)
	})

	commitsBefore := countCommits(t, repo)

	var output bytes.Buffer
	err = run(context.Background(), &output, []string{"-patch"}, nil)
	if err == nil {
		t.Fatalf("expected error from no-match config, got nil\noutput:\n%s", output.String())
	}
	if !strings.Contains(err.Error(), "no match") {
		t.Errorf("expected error to mention 'no match', got: %v", err)
	}

	if got := countCommits(t, repo); got != commitsBefore {
		t.Errorf("commits changed from %d to %d after failed bump", commitsBefore, got)
	}
	exists, err := tagExists(repo, "v1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Errorf("tag v1.0.1 should not exist after failed bump")
	}
}

func TestBumpConfigUnknownPresetAborts(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(originalDir, "testdata", "configs", "unknown-preset.toml")
	_, repo := prepareBumpRepo(t, "v1.0.0", func(tempDir string) {
		cfgContent, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tempDir, ".bump.toml"), cfgContent, 0644); err != nil {
			t.Fatal(err)
		}
	})

	commitsBefore := countCommits(t, repo)

	var output bytes.Buffer
	err = run(context.Background(), &output, []string{"-patch"}, nil)
	if err == nil {
		t.Fatalf("expected error for unknown preset, got nil")
	}
	if !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("expected error to mention 'unknown format', got: %v", err)
	}
	if got := countCommits(t, repo); got != commitsBefore {
		t.Errorf("commits changed %d -> %d on failure", commitsBefore, got)
	}
}

