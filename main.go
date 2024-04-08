package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"golang.org/x/mod/semver"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	// go-git:
	_ "github.com/go-git/go-git/v5"
	_ "golang.org/x/mod/semver"
)

type action int

const (
	none action = iota
	incrementPatch
	incrementMinor
	incrementMajor
)

type config struct {
	version string

	action action
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
	runConfig, showHelp, err := getConfig(argv)
	if err != nil {
		return fmt.Errorf("getConfig: %w", err)
	}
	if showHelp {
		return nil
	}
	versionFilePath := "./.version"

	// Check if the version file exists
	_, err = os.Stat(versionFilePath)
	if os.IsNotExist(err) && runConfig.version == "" {
		return fmt.Errorf("version file does not exist")
	}

	if runConfig.version != "" {
		err := createVersionFile(versionFilePath, runConfig.version)
		if err != nil {
			return fmt.Errorf("createVersionFile: %w", err)
		}
		slog.Info("Created version file", "version", runConfig.version, "path", versionFilePath)
		return nil
	}

	currentVersion, err := readVersionFromFile(versionFilePath)
	if err != nil {
		return fmt.Errorf("readVersionFromFile: %w", err)
	}
	newVersion, err := incrementVersion(currentVersion, runConfig.action)
	if err != nil {
		return fmt.Errorf("incrementVersion: %w", err)
	}
	err = createVersionFile(versionFilePath, newVersion)
	if err != nil {
		return fmt.Errorf("createVersionFile: %w", err)
	}
	err = tagVersion(newVersion)
	if err != nil {
		return fmt.Errorf("tagVersion: %w", err)
	}
	slog.Info("Incremented version", "old", currentVersion, "new", newVersion)
	return nil
}

var semVerRegexp = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

func lastTag(repo *git.Repository) (string, error) {
	// Open the current Git repository
	repo, err := git.PlainOpen(".")
	if err != nil {
		return "", fmt.Errorf("failed to open repository: %w", err)
	}

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
	return cfg, false, nil
}

func createVersionFile(filePath, version string) error {
	err := os.WriteFile(filePath, []byte(version), 0644)
	if err != nil {
		return fmt.Errorf("failed to create version file: %w", err)
	}
	return nil
}

func readVersionFromFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read version file: %w", err)
	}
	return string(content), nil
}

func incrementVersion(currentVersion string, action action) (string, error) {
	parts := strings.Split(currentVersion, ".")
	if len(parts) != 3 {
		log.Fatalf("Invalid version format: %s", currentVersion)
	}

	var major, minor, patch int
	_, err := fmt.Sscanf(currentVersion, "%d.%d.%d", &major, &minor, &patch)
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
		panic("invalid action")
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
}

func tagVersion(version string) error {
	return nil
}
