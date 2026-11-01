package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func loadTestRepository(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read test repository: %w", err)
	}

	var repository map[string][]string

	if err := json.Unmarshal(data, &repository); err != nil {
		return nil, fmt.Errorf("invalid JSON format: %w", err)
	}

	if len(repository) == 0 {
		return nil, fmt.Errorf("test repository is empty")
	}

	for packageName, dependencies := range repository {
		if !isValidPackageName(packageName) {
			return nil, fmt.Errorf("invalid package name: %s", packageName)
		}

		for _, dependency := range dependencies {
			if !isValidPackageName(dependency) {
				return nil, fmt.Errorf("invalid dependency name: %s", dependency)
			}

			if _, exists := repository[dependency]; !exists {
				return nil, fmt.Errorf(
					"package %s depends on undefined package %s",
					packageName,
					dependency,
				)
			}
		}
	}

	return repository, nil
}

func isValidPackageName(name string) bool {
	if name == "" {
		return false
	}

	for _, char := range name {
		if char < 'A' || char > 'Z' {
			return false
		}
	}

	return true
}
