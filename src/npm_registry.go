package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type RegistryMetadata struct {
	DistTags struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`

	Versions map[string]PackageVersion `json:"versions"`
}

type PackageVersion struct {
	Dependencies map[string]string `json:"dependencies"`
}

func fetchPackageMetadata(repositoryURL, packageName string) (RegistryMetadata, error) {
	var metadata RegistryMetadata

	baseURL, err := url.Parse(strings.TrimRight(repositoryURL, "/") + "/")
	if err != nil {
		return metadata, fmt.Errorf("invalid repository URL: %w", err)
	}

	baseURL.Path += packageName
	requestURL := baseURL.String()

	client := http.Client{
		Timeout: 10 * time.Second,
	}

	response, err := client.Get(requestURL)
	if err != nil {
		return metadata, fmt.Errorf("failed to fetch package metadata: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return metadata, fmt.Errorf("registry returned HTTP status %s", response.Status)
	}

	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		return metadata, fmt.Errorf("failed to decode package metadata: %w", err)
	}

	return metadata, nil
}

func extractDirectDependencies(metadata RegistryMetadata) (map[string]string, error) {
	latestVersion := metadata.DistTags.Latest
	if latestVersion == "" {
		return nil, fmt.Errorf("latest package version was not found")
	}

	packageVersion, exists := metadata.Versions[latestVersion]
	if !exists {
		return nil, fmt.Errorf("metadata for latest version %s was not found", latestVersion)
	}

	return packageVersion.Dependencies, nil
}
