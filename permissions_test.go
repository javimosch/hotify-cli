package main

import "testing"

func TestValidatePermission(t *testing.T) {
	tests := []struct {
		name  string
		perm  string
		want  bool
	}{
		{"valid deploy", "deploy", true},
		{"valid start", "start", true},
		{"valid stop", "stop", true},
		{"valid restart", "restart", true},
		{"valid logs", "logs", true},
		{"valid config", "config", true},
		{"valid admin", "admin", true},
		{"wildcard all", "all", true},
		{"wildcard star", "*", true},
		{"empty string", "", false},
		{"unknown permission", "superadmin", false},
		{"whitespace", " deploy ", false},
		{"mixed case", "Deploy", false},
		{"composite", "deploy,start", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidatePermission(tt.perm); got != tt.want {
				t.Errorf("ValidatePermission(%q) = %v, want %v", tt.perm, got, tt.want)
			}
		})
	}
}

func TestValidatePermission_allDefinedConstantsAreValid(t *testing.T) {
	for _, p := range AllPermissions {
		if !ValidatePermission(string(p)) {
			t.Errorf("AllPermissions entry %q should be valid", p)
		}
	}
}

func TestParsePermissions(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Permission
		wantErr bool
	}{
		{
			name:    "empty string",
			input:   "",
			want:    []Permission{},
			wantErr: false,
		},
		{
			name:    "single permission",
			input:   "deploy",
			want:    []Permission{PermissionDeploy},
			wantErr: false,
		},
		{
			name:    "multiple permissions",
			input:   "deploy,start,stop",
			want:    []Permission{PermissionDeploy, PermissionStart, PermissionStop},
			wantErr: false,
		},
		{
			name:    "permissions with whitespace",
			input:   " deploy , start ",
			want:    []Permission{PermissionDeploy, PermissionStart},
			wantErr: false,
		},
		{
			name:    "wildcard all expands to AllPermissions",
			input:   "all",
			want:    AllPermissions,
			wantErr: false,
		},
		{
			name:    "wildcard star expands to AllPermissions",
			input:   "*",
			want:    AllPermissions,
			wantErr: false,
		},
		{
			name:    "invalid permission",
			input:   "deploy,invalid_perm",
			wantErr: true,
		},
		{
			name:    "all expands to AllPermissions even with extra parts",
			input:   "all,nonexistent",
			want:    AllPermissions,
			wantErr: false, // "all" short-circuits and returns AllPermissions immediately
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePermissions(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePermissions(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("ParsePermissions(%q) = %v (len=%d), want %v (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
					return
				}
				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("ParsePermissions(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestPermissionsToString(t *testing.T) {
	tests := []struct {
		name  string
		input []Permission
		want  string
	}{
		{"nil slice", nil, ""},
		{"empty slice", []Permission{}, ""},
		{"single", []Permission{PermissionDeploy}, "deploy"},
		{"multiple", []Permission{PermissionDeploy, PermissionStart, PermissionAdmin}, "deploy,start,admin"},
		{"all permissions", AllPermissions, "deploy,start,stop,restart,logs,config,admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PermissionsToString(tt.input); got != tt.want {
				t.Errorf("PermissionsToString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPermissionsToString_roundTrip(t *testing.T) {
	// Parse then stringify should give back the original string (modulo whitespace)
	original := "deploy,start,logs"
	parsed, err := ParsePermissions(original)
	if err != nil {
		t.Fatalf("ParsePermissions(%q) unexpected error: %v", original, err)
	}
	got := PermissionsToString(parsed)
	if got != original {
		t.Errorf("round-trip: Parse then String = %q, want %q", got, original)
	}
}

func TestMatchWildcardPath(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"exact match", "/api/status", "/api/status", true},
		{"exact mismatch", "/api/status", "/api/config", false},
		{"wildcard matching", "/api/apps/*/start", "/api/apps/myapp/start", true},
		{"wildcard mismatched depth", "/api/apps/*/start", "/api/apps/myapp/sub/start", false},
		{"wildcard different method path", "/api/apps/*/stop", "/api/apps/myapp/start", false},
		{"root wildcard", "/api/api-keys/*", "/api/api-keys/abc123", true},
		{"root wildcard no key", "/api/api-keys/*", "/api/api-keys", false},
		{"different lengths", "/a/b/c", "/a/b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchWildcardPath(tt.pattern, tt.path); got != tt.want {
				t.Errorf("matchWildcardPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestGetRequiredPermissions(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		method string
		want   []Permission
	}{
		{
			name:   "status endpoint",
			path:   "/api/status",
			method: "GET",
			want:   []Permission{PermissionLogs},
		},
		{
			name:   "config GET",
			path:   "/api/config",
			method: "GET",
			want:   []Permission{PermissionConfig},
		},
		{
			name:   "config POST",
			path:   "/api/config",
			method: "POST",
			want:   []Permission{PermissionConfig},
		},
		{
			name:   "wildcard app start",
			path:   "/api/apps/myapp/start",
			method: "POST",
			want:   []Permission{PermissionStart},
		},
		{
			name:   "wildcard app stop",
			path:   "/api/apps/other/stop",
			method: "POST",
			want:   []Permission{PermissionStop},
		},
		{
			name:   "admin endpoints require admin",
			path:   "/api/api-keys",
			method: "GET",
			want:   []Permission{PermissionAdmin},
		},
		{
			name:   "deploy endpoint",
			path:   "/api/deploy",
			method: "POST",
			want:   []Permission{PermissionDeploy},
		},
		{
			name:   "unknown endpoint defaults to admin",
			path:   "/api/unknown",
			method: "GET",
			want:   []Permission{PermissionAdmin},
		},
		{
			name:   "compose deploy",
			path:   "/api/compose/deploy",
			method: "POST",
			want:   []Permission{PermissionDeploy},
		},
		{
			name:   "api keys delete",
			path:   "/api/api-keys/some-key-id",
			method: "DELETE",
			want:   []Permission{PermissionAdmin},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRequiredPermissions(tt.path, tt.method)
			if len(got) != len(tt.want) {
				t.Errorf("GetRequiredPermissions(%q, %q) = %v, want %v", tt.path, tt.method, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("GetRequiredPermissions(%q, %q)[%d] = %q, want %q", tt.path, tt.method, i, got[i], tt.want[i])
				}
			}
		})
	}
}
