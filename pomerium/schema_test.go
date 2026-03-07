package pomerium_test

import (
	"encoding/json"
	"testing"

	pomeriumpkg "github.com/pomerium/mcp-servers/pomerium"
	sdkproto "github.com/pomerium/sdk-go/proto/pomerium"
)

func TestMessageToJSONSchema_GetRouteRequest(t *testing.T) {
	svc := sdkproto.File_config_proto.Services().ByName("ConfigService")
	if svc == nil {
		t.Fatal("ConfigService not found")
	}

	method := svc.Methods().ByName("GetRoute")
	if method == nil {
		t.Fatal("GetRoute method not found")
	}

	schema := pomeriumpkg.MessageToJSONSchema(method.Input())

	// Should be an object
	if schema["type"] != "object" {
		t.Fatalf("expected type=object, got %v", schema["type"])
	}

	// Should have properties
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	// GetRouteRequest should have an "id" field
	if _, ok := props["id"]; !ok {
		t.Error("expected 'id' property in GetRouteRequest schema")
	}

	// Print for debugging
	data, _ := json.MarshalIndent(schema, "", "  ")
	t.Logf("GetRouteRequest schema:\n%s", string(data))
}

func TestMessageToJSONSchema_CreateRouteRequest(t *testing.T) {
	svc := sdkproto.File_config_proto.Services().ByName("ConfigService")
	method := svc.Methods().ByName("CreateRoute")

	schema := pomeriumpkg.MessageToJSONSchema(method.Input())
	data, _ := json.MarshalIndent(schema, "", "  ")
	t.Logf("CreateRouteRequest schema:\n%s", string(data))

	// Should have a "route" property that is an object
	props := schema["properties"].(map[string]any)
	routeProp, ok := props["route"]
	if !ok {
		t.Fatal("expected 'route' property")
	}
	routeSchema, ok := routeProp.(map[string]any)
	if !ok {
		t.Fatal("expected route to be an object schema")
	}
	if routeSchema["type"] != "object" {
		t.Errorf("expected route type=object, got %v", routeSchema["type"])
	}
}

func TestMessageToJSONSchema_GetRouteResponse(t *testing.T) {
	svc := sdkproto.File_config_proto.Services().ByName("ConfigService")
	method := svc.Methods().ByName("GetRoute")

	schema := pomeriumpkg.MessageToJSONSchema(method.Output())
	data, _ := json.MarshalIndent(schema, "", "  ")
	t.Logf("GetRouteResponse schema:\n%s", string(data))

	props := schema["properties"].(map[string]any)
	if _, ok := props["route"]; !ok {
		t.Error("expected 'route' property in GetRouteResponse")
	}
}

func TestMessageToJSONSchema_ListRoutesRequest(t *testing.T) {
	svc := sdkproto.File_config_proto.Services().ByName("ConfigService")
	method := svc.Methods().ByName("ListRoutes")

	schema := pomeriumpkg.MessageToJSONSchema(method.Input())
	data, _ := json.MarshalIndent(schema, "", "  ")
	t.Logf("ListRoutesRequest schema:\n%s", string(data))
}
