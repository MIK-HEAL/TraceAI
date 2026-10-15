package distribution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type versionManifest struct {
	Version   string   `json:"version"`
	Release   string   `json:"release_tag"`
	GoModule  string   `json:"go_module"`
	Npm       string   `json:"npm_package"`
	Python    string   `json:"python_package"`
	Platforms []string `json:"supported_platforms"`
	Arches    []string `json:"supported_architectures"`
}

func TestDistributionVersionsStayAligned(t *testing.T) {
	data, err := os.ReadFile("version.json")
	if err != nil {
		t.Fatal(err)
	}
	input := strings.TrimPrefix(string(data), "\ufeff")
	var manifest versionManifest
	if err := json.Unmarshal([]byte(input), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version == "" || manifest.Release == "" {
		t.Fatal("version manifest is incomplete")
	}
	if manifest.Release != "v"+manifest.Version {
		t.Fatalf("release tag must match version: %s vs v%s", manifest.Release, manifest.Version)
	}
	if !contains(manifest.Platforms, "linux") || !contains(manifest.Platforms, "darwin") || !contains(manifest.Platforms, "windows") {
		t.Fatalf("release manifest must include linux, darwin, and windows: %v", manifest.Platforms)
	}
	if !contains(manifest.Arches, "amd64") || !contains(manifest.Arches, "arm64") {
		t.Fatalf("release manifest must include amd64 and arm64: %v", manifest.Arches)
	}

	npmData, err := os.ReadFile(filepath.Join("npm", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	npmVersion := findVersion(t, strings.TrimPrefix(string(npmData), "\ufeff"))
	if npmVersion != manifest.Version {
		t.Fatalf("npm version mismatch: %s vs %s", npmVersion, manifest.Version)
	}

	pyData, err := os.ReadFile(filepath.Join("python", "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	pyVersion := findPythonVersion(t, strings.TrimPrefix(string(pyData), "\ufeff"))
	if pyVersion != manifest.Version {
		t.Fatalf("python version mismatch: %s vs %s", pyVersion, manifest.Version)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func findVersion(t *testing.T, input string) string {
	t.Helper()
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(input), &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Version == "" {
		t.Fatal("version field not found")
	}
	return pkg.Version
}

func findPythonVersion(t *testing.T, input string) string {
	t.Helper()
	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `version = "`) {
			value := strings.TrimPrefix(line, `version = "`)
			value = strings.TrimSuffix(value, `"`)
			if value != "" {
				return value
			}
		}
	}
	t.Fatal("python version field not found")
	return ""
}
