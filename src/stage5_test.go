package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIsValidPackageName(t *testing.T) {
	tests := []struct {
		name string
		valid bool
	}{
		{"A", true},
		{"ABC", true},
		{"", false},
		{"abc", false},
		{"AbC", false},
		{"A1", false},
		{"A-B", false},
		{"А", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidPackageName(tt.name); got != tt.valid {
				t.Fatalf("isValidPackageName(%q) = %v, want %v", tt.name, got, tt.valid)
			}
		})
	}
}

func TestLoadTestRepository(t *testing.T) {
	t.Run("valid repository", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "repo.json")
		data := `{"A":["B"],"B":[]}`
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := loadTestRepository(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string][]string{"A": {"B"}, "B": {}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	tests := []struct {
		name string
		data string
		wantErr string
	}{
		{"invalid JSON", `{`, "invalid JSON format"},
		{"empty repository", `{}`, "test repository is empty"},
		{"invalid package name", `{"a":[]}`, "invalid package name"},
		{"invalid dependency name", `{"A":["b"]}`, "invalid dependency name"},
		{"undefined dependency", `{"A":["B"]}`, "depends on undefined package"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "repo.json")
			if err := os.WriteFile(path, []byte(tt.data), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := loadTestRepository(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		_, err := loadTestRepository(filepath.Join(t.TempDir(), "missing.json"))
		if err == nil || !strings.Contains(err.Error(), "failed to read test repository") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateRepositoryURL(t *testing.T) {
	valid := []string{"https://registry.npmjs.org/", "http://localhost:4873"}
	for _, value := range valid {
		if err := validateRepositoryURL(value); err != nil {
			t.Errorf("validateRepositoryURL(%q): %v", value, err)
		}
	}
	invalid := []string{"", "registry.npmjs.org", "ftp://example.com", "https:///missing-host"}
	for _, value := range invalid {
		if err := validateRepositoryURL(value); err == nil {
			t.Errorf("validateRepositoryURL(%q) should fail", value)
		}
	}
}

func TestValidateTestRepository(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "repo.json")
	if err := os.WriteFile(file, []byte(`{"A":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateTestRepository(file); err != nil {
		t.Fatalf("valid file rejected: %v", err)
	}
	if err := validateTestRepository(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing file should be rejected")
	}
	if err := validateTestRepository(dir); err == nil {
		t.Fatal("directory should be rejected")
	}
}

func TestValidateConfig(t *testing.T) {
	validFile := filepath.Join(t.TempDir(), "repo.json")
	if err := os.WriteFile(validFile, []byte(`{"A":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		config Config
		wantErr string
	}{
		{"valid test config", Config{Package: "A", Repository: validFile, TestMode: true, MaxDepth: 3}, ""},
		{"missing package", Config{Repository: validFile, TestMode: true}, "parameter --package is required"},
		{"missing repository", Config{Package: "A", TestMode: true}, "parameter --repository is required"},
		{"negative depth", Config{Package: "A", Repository: validFile, TestMode: true, MaxDepth: -1}, "--max-depth cannot be negative"},
		{"invalid URL", Config{Package: "A", Repository: "ftp://example.com"}, "valid HTTP or HTTPS URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	oldArgs, oldCommandLine := os.Args, flag.CommandLine
	defer func() { os.Args, flag.CommandLine = oldArgs, oldCommandLine }()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	os.Args = []string{"app"}
	got := parseConfig()
	if got.MaxDepth != 3 || got.TestMode || got.ASCIITree || got.Package != "" || got.Repository != "" {
		t.Fatalf("unexpected defaults: %+v", got)
	}

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	os.Args = []string{"app", "--package", "A", "--repository", "repo.json", "--test-mode", "--ascii-tree", "--max-depth", "5"}
	got = parseConfig()
	want := Config{Package: "A", Repository: "repo.json", TestMode: true, ASCIITree: true, MaxDepth: 5}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBuildDependencyGraph(t *testing.T) {
	repository := map[string][]string{
		"A": {"B", "C"}, "B": {"D"}, "C": {"D", "E"},
		"D": {}, "E": {},
	}
	get := func(name string) ([]string, error) { return repository[name], nil }

	graph, cycles, err := buildDependencyGraph("A", 10, get)
	if err != nil {
		t.Fatal(err)
	}
	if len(cycles) != 0 {
		t.Fatalf("unexpected cycles: %v", cycles)
	}
	want := map[string][]string{"A": {"B", "C"}, "B": {"D"}, "C": {"D", "E"}, "D": {}, "E": {}}
	if !reflect.DeepEqual(graph, want) {
		t.Fatalf("graph = %#v, want %#v", graph, want)
	}
}

func TestBuildDependencyGraphDepthLimit(t *testing.T) {
	repository := map[string][]string{"A": {"B"}, "B": {"C"}, "C": {"D"}, "D": {}}
	get := func(name string) ([]string, error) { return repository[name], nil }

	graph, _, err := buildDependencyGraph("A", 2, get)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"A": {"B"}, "B": {"C"}, "C": {}}
	if !reflect.DeepEqual(graph, want) {
		t.Fatalf("graph = %#v, want %#v", graph, want)
	}
}

func TestBuildDependencyGraphCycle(t *testing.T) {
	repository := map[string][]string{"A": {"B"}, "B": {"C"}, "C": {"A"}}
	get := func(name string) ([]string, error) { return repository[name], nil }
	_, cycles, err := buildDependencyGraph("A", 10, get)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cycles, []string{"C -> A"}) {
		t.Fatalf("cycles = %v, want [C -> A]", cycles)
	}
}

func TestBuildDependencyGraphDependencyError(t *testing.T) {
	wantErr := errors.New("registry unavailable")
	get := func(name string) ([]string, error) {
		if name == "B" { return nil, wantErr }
		return []string{"B"}, nil
	}
	_, _, err := buildDependencyGraph("A", 5, get)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
}

func TestBuildDependencyGraphZeroDepth(t *testing.T) {
	calls := 0
	get := func(string) ([]string, error) { calls++; return nil, nil }
	graph, cycles, err := buildDependencyGraph("A", 0, get)
	if err != nil { t.Fatal(err) }
	if calls != 0 { t.Fatalf("getDependencies called %d times, want 0", calls) }
	if len(cycles) != 0 || !reflect.DeepEqual(graph, map[string][]string{"A": {}}) {
		t.Fatalf("unexpected result: graph=%v cycles=%v", graph, cycles)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	os.Stdout = writer
	fn()
	_ = writer.Close()
	os.Stdout = old
	var output bytes.Buffer
	if _, err := io.Copy(&output, reader); err != nil { t.Fatal(err) }
	_ = reader.Close()
	return output.String()
}

func TestPrintDependencyTree(t *testing.T) {
	graph := map[string][]string{"A": {"B", "C"}, "B": {"D"}, "C": {}, "D": {}}
	got := captureStdout(t, func() { printDependencyTree("A", graph) })
	want := "A\n├── B\n│   └── D\n└── C\n"
	if got != want {
		t.Fatalf("tree output:\n%q\nwant:\n%q", got, want)
	}
}

func TestPrintDependencyTreeCycle(t *testing.T) {
	graph := map[string][]string{"A": {"B"}, "B": {"A"}}
	got := captureStdout(t, func() { printDependencyTree("A", graph) })
	if !strings.Contains(got, "A (cycle)") {
		t.Fatalf("expected cycle marker, got %q", got)
	}
}

func TestGenerateD2Graph(t *testing.T) {
	graph := map[string][]string{"C": {"E", "D"}, "A": {"C", "B"}, "B": {"D"}, "D": {}, "E": {}}
	got := generateD2Graph(graph)
	want := "A -> B\nA -> C\nB -> D\nC -> D\nC -> E\n"
	if got != want {
		t.Fatalf("D2 output = %q, want %q", got, want)
	}
}

func TestGenerateD2GraphEmpty(t *testing.T) {
	if got := generateD2Graph(map[string][]string{"A": {}}); got != "" {
		t.Fatalf("empty graph output = %q, want empty string", got)
	}
}

func TestSaveD2Graph(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.d2")
	content := "A -> B\n"
	if err := saveD2Graph(content, path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	if string(got) != content {
		t.Fatalf("file content = %q, want %q", got, content)
	}
}

func TestSaveD2GraphInvalidPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "graph.d2")
	if err := saveD2Graph("A -> B\n", path); err == nil {
		t.Fatal("expected error for invalid output path")
	}
}

func TestFetchPackageMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/debug" { t.Errorf("request path = %q, want /debug", r.URL.Path) }
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dist-tags": map[string]string{"latest": "4.4.3"},
			"versions": map[string]any{"4.4.3": map[string]any{"dependencies": map[string]string{"ms": "^2.1.3"}}},
		})
	}))
	defer server.Close()

	got, err := fetchPackageMetadata(server.URL, "debug")
	if err != nil { t.Fatal(err) }
	if got.DistTags.Latest != "4.4.3" || got.Versions["4.4.3"].Dependencies["ms"] != "^2.1.3" {
		t.Fatalf("unexpected metadata: %+v", got)
	}
}

func TestFetchPackageMetadataHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()
	_, err := fetchPackageMetadata(server.URL, "missing")
	if err == nil || !strings.Contains(err.Error(), "HTTP status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchPackageMetadataInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "{invalid")
	}))
	defer server.Close()
	_, err := fetchPackageMetadata(server.URL, "debug")
	if err == nil || !strings.Contains(err.Error(), "failed to decode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractDirectDependencies(t *testing.T) {
	var metadata RegistryMetadata
	metadata.DistTags.Latest = "1.0.0"
	metadata.Versions = map[string]PackageVersion{
		"1.0.0": {Dependencies: map[string]string{"alpha": "^1.0.0", "beta": "~2.0.0"}},
	}
	got, err := extractDirectDependencies(metadata)
	if err != nil { t.Fatal(err) }
	want := map[string]string{"alpha": "^1.0.0", "beta": "~2.0.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dependencies = %v, want %v", got, want)
	}
}

func TestExtractDirectDependenciesErrors(t *testing.T) {
	var noLatest RegistryMetadata
	if _, err := extractDirectDependencies(noLatest); err == nil {
		t.Fatal("expected error when latest version is missing")
	}
	var missingVersion RegistryMetadata
	missingVersion.DistTags.Latest = "2.0.0"
	missingVersion.Versions = map[string]PackageVersion{}
	if _, err := extractDirectDependencies(missingVersion); err == nil {
		t.Fatal("expected error when latest version metadata is missing")
	}
}


func TestRenderD2GraphMissingExecutable(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "graph.d2")
	if err := os.WriteFile(inputPath, []byte("A -> B\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Use an empty PATH so the test does not depend on D2 being installed.
	t.Setenv("PATH", t.TempDir())

	err := renderD2Graph(inputPath, filepath.Join(t.TempDir(), "graph.svg"))
	if err == nil || !strings.Contains(err.Error(), "failed to render D2 graph") {
		t.Fatalf("expected a D2 execution error, got %v", err)
	}
}
