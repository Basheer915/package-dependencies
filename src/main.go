package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

type Config struct {
	Package    string
	Repository string
	TestMode   bool
	ASCIITree  bool
	MaxDepth   int
}

func parseConfig() Config {
	config := Config{}

	flag.StringVar(
		&config.Package,
		"package",
		"",
		"Name of the npm package to analyze",
	)

	flag.StringVar(
		&config.Repository,
		"repository",
		"",
		"Repository URL or test repository file path",
	)

	flag.BoolVar(
		&config.TestMode,
		"test-mode",
		false,
		"Use a test repository file",
	)

	flag.BoolVar(
		&config.ASCIITree,
		"ascii-tree",
		false,
		"Output dependencies as an ASCII tree",
	)

	flag.IntVar(
		&config.MaxDepth,
		"max-depth",
		3,
		"Maximum dependency analysis depth",
	)

	flag.Parse()

	return config
}

func validateConfig(config Config) error {
	if strings.TrimSpace(config.Package) == "" {
		return fmt.Errorf("parameter --package is required")
	}

	if strings.TrimSpace(config.Repository) == "" {
		return fmt.Errorf("parameter --repository is required")
	}

	if config.MaxDepth < 0 {
		return fmt.Errorf("--max-depth cannot be negative")
	}

	if config.TestMode {
		return validateTestRepository(config.Repository)
	}

	return validateRepositoryURL(config.Repository)
}

func validateTestRepository(path string) error {
	info, err := os.Stat(path)

	if err != nil {
		return fmt.Errorf("invalid test repository file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("test repository must be a regular file")
	}

	return nil
}

func validateRepositoryURL(value string) error {
	parsed, err := url.ParseRequestURI(value)

	if err != nil ||
		parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf(
			"--repository must be a valid HTTP or HTTPS URL",
		)
	}

	return nil
}

func main() {
	config := parseConfig()

	if err := validateConfig(config); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	var getDependencies func(string) ([]string, error)

	if config.TestMode {
		repository, err := loadTestRepository(config.Repository)

		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

		if _, exists := repository[config.Package]; !exists {
			fmt.Fprintln(
				os.Stderr,
				"Error: root package not found in test repository",
			)
			os.Exit(1)
		}

		getDependencies = func(packageName string) ([]string, error) {
			return repository[packageName], nil
		}

	} else {
		getDependencies = func(packageName string) ([]string, error) {
			metadata, err := fetchPackageMetadata(
				config.Repository,
				packageName,
			)

			if err != nil {
				return nil, err
			}

			dependencies, err := extractDirectDependencies(metadata)

			if err != nil {
				return nil, err
			}

			names := make([]string, 0, len(dependencies))

			for name := range dependencies {
				names = append(names, name)
			}

			sort.Strings(names)

			return names, nil
		}
	}

	graph, cycles, err := buildDependencyGraph(
		config.Package,
		config.MaxDepth,
		getDependencies,
	)

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	fmt.Printf("Dependency tree of %s:\n", config.Package)
	printDependencyTree(config.Package, graph)

	if len(cycles) > 0 {
		fmt.Println("\nDetected cycles:")

		for _, cycle := range cycles {
			fmt.Println(cycle)
		}
	}
}
