package pomerium_test

import (
	"testing"

	pomeriumpkg "github.com/pomerium/mcp-servers/pomerium"
	sdkproto "github.com/pomerium/sdk-go/proto/pomerium"
)

func TestCamelToSnake(t *testing.T) {
	tests := map[string]string{
		"GetRoute":             "get_route",
		"ListRoutes":           "list_routes",
		"CreateServiceAccount": "create_service_account",
		"DeleteKeyPair":        "delete_key_pair",
		"UpdateSettings":       "update_settings",
		"GetServerInfo":        "get_server_info",
	}
	for input, want := range tests {
		got := pomeriumpkg.CamelToSnake(input)
		if got != want {
			t.Errorf("CamelToSnake(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDiscoverMethods(t *testing.T) {
	svcs := sdkproto.File_config_proto.Services()
	if svcs.Len() == 0 {
		t.Fatal("no services found in File_config_proto")
	}

	svc := svcs.ByName("ConfigService")
	if svc == nil {
		t.Fatal("ConfigService not found")
	}

	methods := svc.Methods()
	t.Logf("ConfigService has %d methods:", methods.Len())

	expectedMethods := []string{
		"GetRoute", "ListRoutes", "CreateRoute", "UpdateRoute", "DeleteRoute",
		"GetPolicy", "ListPolicies", "CreatePolicy", "UpdatePolicy", "DeletePolicy",
		"GetSettings", "UpdateSettings", "ListSettings",
		"GetServiceAccount", "ListServiceAccounts", "CreateServiceAccount", "UpdateServiceAccount", "DeleteServiceAccount",
		"GetKeyPair", "ListKeyPairs", "CreateKeyPair", "UpdateKeyPair", "DeleteKeyPair",
		"GetServerInfo",
	}

	found := make(map[string]bool)
	for i := range methods.Len() {
		m := methods.Get(i)
		name := string(m.Name())
		found[name] = true
		t.Logf("  %s (streaming_client=%v, streaming_server=%v)",
			name, m.IsStreamingClient(), m.IsStreamingServer())
	}

	for _, expected := range expectedMethods {
		if !found[expected] {
			t.Errorf("expected method %s not found", expected)
		}
	}
}

func TestAnnotationsForMethod(t *testing.T) {
	tests := []struct {
		method   string
		readOnly bool
		destrNil bool // destructiveHint is nil (for read-only)
	}{
		{"GetRoute", true, true},
		{"ListRoutes", true, true},
		{"CreateRoute", false, false},
		{"UpdateRoute", false, false},
		{"DeleteRoute", false, false},
	}

	for _, tt := range tests {
		ann := pomeriumpkg.AnnotationsForMethod(tt.method)
		if ann.ReadOnlyHint != tt.readOnly {
			t.Errorf("%s: ReadOnlyHint = %v, want %v", tt.method, ann.ReadOnlyHint, tt.readOnly)
		}
		if tt.destrNil && ann.DestructiveHint != nil {
			t.Errorf("%s: DestructiveHint should be nil for read-only", tt.method)
		}
	}
}
