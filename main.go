package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"golang.org/x/mod/semver"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

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
}

const (
	versionFilePath = ".version"
)

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
	if runConfig.version != "" {
		err = createVersionFile(versionFilePath, runConfig.version)
		if err != nil {
			return fmt.Errorf("createVersionFile: %w", err)
		}
		tag, err := tagVersion(repo, runConfig.version)
		if err != nil {
			return fmt.Errorf("tagVersion: %w", err)
		}
		_, _ = fmt.Fprintf(output, "Wrote version file '%s' version=%s, tag=%s\n", versionFilePath, runConfig.version, tag.Hash().String())
		return nil
	}
	// increment version
	currentVersion, err := lastTag(repo)
	if err != nil {
		return fmt.Errorf("failed to get last tag: %w", err)
	}

	newVersion, err := incrementVersion(currentVersion, runConfig.action)
	if err != nil {
		return fmt.Errorf("incrementVersion: %w", err)
	}
	err = createVersionFile(versionFilePath, newVersion)
	if err != nil {
		return fmt.Errorf("createVersionFile: %w", err)
	}
	tag, err := tagVersion(repo, newVersion)
	if err != nil {
		return fmt.Errorf("tagVersion: %w", err)
	}
	_, _ = fmt.Fprintf(output, "Wrote version file '%s' version=%s, tag=%s\n", versionFilePath, newVersion, tag.Hash().String())
	return nil
}

var semVerRegexp = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

func lastTag(repo *git.Repository) (string, error) {
	// Get the list of tags
	tagRefs, err := repo.Tags()
	if err != nil {
		return "", fmt.Errorf("failed to get tags: %w", err)
	}
	var tags []string
	err = tagRefs.ForEach(func(t *plumbing.Reference) error {
		// check that the tag matches the semver format
		if !semVerRegexp.MatchString(t.Name().Short()) {
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

	flagSet := flag.NewFlagSet("version", flag.ContinueOnError)
	flagSet.StringVar(&cfg.version, "version", "", "Initial version number.")
	flagSet.BoolVar(&patchFlag, "patch", false, "Increase patch version.")
	flagSet.BoolVar(&minorFlag, "minor", false, "Increase minor version.")
	flagSet.BoolVar(&majorFlag, "major", false, "Increase major version.")
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

	// if both version and increment flags are set, return an error
	if cfg.version != "" && (patchFlag || minorFlag || majorFlag) {
		return config{}, false, fmt.Errorf("cannot set version and increment flags at the same time")
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
	// no action not version given: increment patch
	if cfg.action == noAction && cfg.version == "" {
		cfg.action = incrementPatch
	}
	return cfg, false, nil
}

func createVersionFile(filePath, version string) error {
	err := os.WriteFile(filePath, []byte(version), 0644)
	if err != nil {
		return fmt.Errorf("failed to create version file: %w", err)
	}
	return nil
}

func incrementVersion(currentVersion string, action action) (string, error) {
	parts := strings.Split(currentVersion, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid version format: %s", currentVersion)
	}

	var major, minor, patch int
	_, err := fmt.Sscanf(currentVersion, "v%d.%d.%d", &major, &minor, &patch)
	if err != nil {
		return "", fmt.Errorf("failed to parse current version('%s'): %w", currentVersion, err)
	}
	switch action {
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
		return "", fmt.Errorf("invalid action: %d", action)
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch), nil
}

func tagVersion(repo *git.Repository, version string) (*plumbing.Reference, error) {
	// find the current commit
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}
	opts := &git.CreateTagOptions{
		Message: "tag created by bump",
	}
	return repo.CreateTag(version, head.Hash(), opts)
}
