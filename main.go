package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"golang.org/x/mod/semver"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed .version
var embeddedVersion string

type action int

const (
	noAction action = iota
	incrementPatch
	incrementMinor
	incrementMajor
)

type config struct {
	version string
	action  action
	dryRun  bool
	forced  bool
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	err := run(ctx, os.Stdout, os.Args[1:], os.Environ())
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, output io.Writer, argv []string, env []string) error {
	_, _ = fmt.Fprintf(output, "bump '%s'\n", embeddedVersion)
	runConfig, showHelp, err := getConfig(argv)
	if err != nil {
		return fmt.Errorf("getConfig: %w", err)
	}
	if showHelp {
		return nil
	}

	repo, err := git.PlainOpen(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}
	// check that the repository is clean
	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("repo.Worktree: %w", err)
	}
	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("worktree.Status: %w", err)
	}
	if !status.IsClean() && !runConfig.forced {
		return fmt.Errorf("repository is not clean")
	}

	if runConfig.version != "" {
		err = updateVersionFiles(repo, runConfig, output)
		if err != nil {
			return fmt.Errorf("updateVersionFiles: %w", err)
		}
		hash, err := tagVersion(repo, runConfig)
		if err != nil {
			return fmt.Errorf("tagVersion: %w", err)
		}
		_, _ = fmt.Fprintf(output, "Set embeddedVersion %s, tag=%s\n", runConfig.version, hash)
		return nil
	}
	// increment embeddedVersion
	currentVersion, err := lastTag(repo)
	if err != nil {
		return fmt.Errorf("failed to get last tag: %w", err)
	}

	newVersion, err := incrementVersion(currentVersion, runConfig)
	if err != nil {
		return fmt.Errorf("incrementVersion: %w", err)
	}
	err = updateVersionFiles(repo, runConfig, output)
	if err != nil {
		return fmt.Errorf("updateVersionFiles: %w", err)
	}
	tag, err := tagVersion(repo, runConfig)
	if err != nil {
		return fmt.Errorf("tagVersion: %w", err)
	}
	_, _ = fmt.Fprintf(output, "Bumped embeddedVersion %s --> %s, tag=%s\n", currentVersion,
		newVersion, tag)
	return nil
}

func lastTag(repo *git.Repository) (string, error) {
	// Get the list of tags
	tagRefs, err := repo.Tags()
	if err != nil {
		return "", fmt.Errorf("failed to get tags: %w", err)
	}
	var tags []string
	err = tagRefs.ForEach(func(t *plumbing.Reference) error {
		// check that the tag matches the semver format
		if !semver.IsValid(t.Name().Short()) {
			return nil
		}
		tags = append(tags, t.Name().Short())
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to iterate over tags: %w", err)
	}
	// sort the tags
	semver.Sort(tags)
	// return the last tag
	return tags[len(tags)-1], nil
}

func getConfig(args []string) (config, bool, error) {
	var cfg config
	var showhelp, patchFlag, minorFlag, majorFlag bool

	flagSet := flag.NewFlagSet("embeddedVersion", flag.ContinueOnError)
	flagSet.StringVar(&cfg.version, "embeddedVersion", "", "Initial embeddedVersion number.")
	flagSet.BoolVar(&patchFlag, "patch", false, "Increase patch embeddedVersion.")
	flagSet.BoolVar(&minorFlag, "minor", false, "Increase minor embeddedVersion.")
	flagSet.BoolVar(&majorFlag, "major", false, "Increase major embeddedVersion.")
	flagSet.BoolVar(&cfg.dryRun, "dry-run", false, "Do not write changes to the repository.")
	flagSet.BoolVar(&cfg.forced, "force", false, "Force the action despite the repository being dirty.")
	flagSet.BoolVar(&showhelp, "help", false, "Show help message.")

	err := flagSet.Parse(args)
	if err != nil {
		return config{}, false, fmt.Errorf("failed to parse flags: %w", err)
	}
	if showhelp {
		flagSet.Usage()
		return config{}, true, nil
	}
	// check if there are any arguments left
	if flagSet.NArg() > 0 {
		return config{}, false, fmt.Errorf("unexpected arguments: %s", flagSet.Args())
	}

	// if both embeddedVersion and increment flags are set, return an error
	if cfg.version != "" && (patchFlag || minorFlag || majorFlag) {
		return config{}, false, fmt.Errorf("cannot set embeddedVersion and increment flags at the same time")
	}
	// check that not more than one flag is set:
	if (patchFlag && minorFlag) || (patchFlag && majorFlag) || (minorFlag && majorFlag) {
		return config{}, false, fmt.Errorf("cannot set more than one increment flag at the same time")
	}
	if patchFlag {
		cfg.action = incrementPatch
	}
	if minorFlag {
		cfg.action = incrementMinor
	}
	if majorFlag {
		cfg.action = incrementMajor
	}
	// no action not embeddedVersion given: increment patch
	if cfg.action == noAction && cfg.version == "" {
		cfg.action = incrementPatch
	}
	return cfg, false, nil
}

func updateVersionFiles(repo *git.Repository, cfg config, output io.Writer) error {
	// find all the files name ".embeddedVersion"
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != ".embeddedVersion" {
			return nil
		}
		// read the content of the file
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		// content must either by empty or a valid semver, if not we return an error

		if len(content) > 0 && !semver.IsValid(string(content)) {
			return fmt.Errorf("invalid embeddedVersion in file %s: %s", path, content)
		}
		// print the action to the output.
		_, _ = fmt.Fprintf(output, "Updating embeddedVersion in file %s to %s\n", path, cfg.version)

		if cfg.dryRun {
			return nil // return early if we are in dry-run mode
		}
		// write the new embeddedVersion to the file
		err = os.WriteFile(path, []byte(cfg.version), 0644)
		if err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		// add the file to the repository
		err = add(repo, path)
		if err != nil {
			return fmt.Errorf("failed to add file: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}
	// commit the changes
	err = commit(repo, fmt.Sprintf("bump embeddedVersion to %s", cfg.version))
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func incrementVersion(currentVersion string, cfg config) (string, error) {
	parts := strings.Split(currentVersion, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid embeddedVersion format: %s", currentVersion)
	}

	var major, minor, patch int
	_, err := fmt.Sscanf(currentVersion, "v%d.%d.%d", &major, &minor, &patch)
	if err != nil {
		return "", fmt.Errorf("failed to parse current embeddedVersion('%s'): %w", currentVersion, err)
	}
	switch cfg.action {
	case incrementPatch:
		patch++
	case incrementMinor:
		minor++
		patch = 0
	case incrementMajor:
		major++
		minor = 0
		patch = 0
	default:
		return "", fmt.Errorf("invalid action: %d", cfg.action)
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch), nil
}

func tagVersion(repo *git.Repository, cfg config) (string, error) {
	// find the current commit
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	opts := &git.CreateTagOptions{
		Message: "tag created by bump",
	}
	if cfg.dryRun {
		return head.Hash().String(), nil
	}
	ref, err := repo.CreateTag(cfg.version, head.Hash(), opts)
	if err != nil {
		return "", fmt.Errorf("failed to create tag: %w", err)
	}
	return ref.Hash().String(), nil
}

// add adds the file at the given path to the repository
func add(repo *git.Repository, path string) error {
	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("repo.Worktree: %w", err)
	}
	_, err = w.Add(path)
	if err != nil {
		return fmt.Errorf("worktree.Add(%s): %w", path, err)
	}
	return nil
}

func commit(repo *git.Repository, message string) error {
	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("repo.Worktree: %w", err)
	}
	_, err = w.Commit(message, &git.CommitOptions{})
	if err != nil {
		return fmt.Errorf("worktree.Commit: %w", err)
	}
	return nil
}
