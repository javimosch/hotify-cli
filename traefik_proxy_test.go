package main

import (
	"strings"
	"testing"
)

// TestBuildDynamicYAML_PathPrefixAndBackendURL verifies that proxy service
// attributes are rendered correctly: backend_url becomes the load balancer
// target and path_prefix becomes an addPrefix middleware applied to the router.
func TestBuildDynamicYAML_PathPrefixAndBackendURL(t *testing.T) {
	config := &Config{
		Apps: []App{
			{
				ID:         "slv2",
				Name:       "SL v2",
				Domain:     "slv2.example.com",
				Port:       3100,
				Command:    "slv2 start",
				BackendURL: "http://100.114.4.57:3100",
				PathPrefix: "/slv2",
			},
		},
	}

	yaml, err := buildDynamicYAML(config)
	if err != nil {
		t.Fatalf("buildDynamicYAML failed: %v", err)
	}

	if !strings.Contains(yaml, `url: "http://100.114.4.57:3100"`) {
		t.Errorf("missing backend_url in service; got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "slv2-addprefix") {
		t.Errorf("missing addPrefix middleware reference; got:\n%s", yaml)
	}
	if !strings.Contains(yaml, `prefix: "/slv2"`) {
		t.Errorf("missing addPrefix prefix value; got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "middlewares:") {
		t.Errorf("router should declare middlewares when path_prefix is set; got:\n%s", yaml)
	}
}
