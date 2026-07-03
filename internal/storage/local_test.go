package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDestPath(t *testing.T) {
	baseDir := t.TempDir()

	tests := []struct {
		name    string
		key     string
		wantErr string
	}{
		{name: "valid", key: "2026/616/0.png"},
		{name: "empty", key: "", wantErr: "empty"},
		{name: "dotdot", key: "2026/../secret.png", wantErr: ".."},
		{name: "absolute unix", key: "/etc/passwd", wantErr: "relative"},
		{name: "absolute windows", key: `\windows\system32\config\sam`, wantErr: "forward slashes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest, err := resolveDestPath(baseDir, tt.key)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			baseAbs, err := filepath.Abs(baseDir)
			if err != nil {
				t.Fatalf("abs base dir: %v", err)
			}
			destAbs, err := filepath.Abs(dest)
			if err != nil {
				t.Fatalf("abs dest: %v", err)
			}
			rel, err := filepath.Rel(baseAbs, destAbs)
			if err != nil || strings.HasPrefix(rel, "..") {
				t.Fatalf("resolved path escapes base dir: dest=%s rel=%s err=%v", dest, rel, err)
			}
		})
	}
}
