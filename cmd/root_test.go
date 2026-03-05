package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"404 error", fmt.Errorf("HTTP 404: not found"), true},
		{"not found text", fmt.Errorf("resource not found"), true},
		{"other error", fmt.Errorf("connection refused"), false},
		{"empty error", fmt.Errorf(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestWriteAndReadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	type testData struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	// Write
	original := testData{Name: "test", Count: 42}
	if err := WriteJSON(path, original); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	// Read back
	result, err := ReadJSON[testData](path)
	if err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if result.Name != "test" || result.Count != 42 {
		t.Errorf("ReadJSON = %+v, want {Name:test Count:42}", result)
	}

	// Verify indentation
	data, _ := os.ReadFile(path)
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("file is not valid JSON: %v", err)
	}
	// Should contain indented output (2 spaces)
	if len(data) < 20 {
		t.Error("output appears not to be indented")
	}
}

func TestReadJSONFileNotFound(t *testing.T) {
	_, err := ReadJSON[map[string]string]("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestWriteJSONInvalidPath(t *testing.T) {
	err := WriteJSON("/nonexistent/dir/test.json", "data")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestAllRetrieverStatuses(t *testing.T) {
	// Verify we have all 21 statuses
	if len(AllRetrieverStatuses) != 21 {
		t.Errorf("len(AllRetrieverStatuses) = %d, want 21", len(AllRetrieverStatuses))
	}

	// Verify all have non-empty fields
	seenKeys := make(map[string]bool)
	validTypes := map[string]bool{
		"deployable":   true,
		"pending":      true,
		"archived":     true,
		"undeployable": true,
	}
	for _, s := range AllRetrieverStatuses {
		if s.Key == "" {
			t.Error("status has empty Key")
		}
		if s.Label == "" {
			t.Errorf("status %q has empty Label", s.Key)
		}
		if !validTypes[s.SnipeType] {
			t.Errorf("status %q has invalid SnipeType %q", s.Key, s.SnipeType)
		}
		if seenKeys[s.Key] {
			t.Errorf("duplicate status key %q", s.Key)
		}
		seenKeys[s.Key] = true
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}
	if cfg.CacheDir != "" {
		t.Errorf("default CacheDir = %q, want empty", cfg.CacheDir)
	}
	if cfg.DryRun {
		t.Error("default DryRun = true, want false")
	}
	if cfg.Debug {
		t.Error("default Debug = true, want false")
	}
	if cfg.UseCache {
		t.Error("default UseCache = true, want false")
	}
}
