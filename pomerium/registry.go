package pomerium

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// skipMethods lists methods that should not be exposed as MCP tools.
var skipMethods = map[string]bool{
	"GetServerInfo": true, // internal, used by auth transport
}

// RegisterTools walks a protobuf FileDescriptor's services and registers
// each RPC method as an MCP tool on the server.
func RegisterTools(
	s *mcp.Server,
	caller *DynamicCaller,
	fileDesc protoreflect.FileDescriptor,
) {
	svcs := fileDesc.Services()
	for i := range svcs.Len() {
		svc := svcs.Get(i)
		methods := svc.Methods()
		for j := range methods.Len() {
			method := methods.Get(j)
			methodName := string(method.Name())

			if skipMethods[methodName] {
				continue
			}

			// Skip streaming methods — MCP tools are unary
			if method.IsStreamingClient() || method.IsStreamingServer() {
				continue
			}

			registerMethod(s, caller, method)
		}
	}
}

func registerMethod(s *mcp.Server, caller *DynamicCaller, method protoreflect.MethodDescriptor) {
	methodName := string(method.Name())
	toolName := CamelToSnake(methodName)

	inputSchema := MessageToJSONSchema(method.Input())
	outputSchema := MessageToJSONSchema(method.Output())
	annotations := AnnotationsForMethod(methodName)

	tool := &mcp.Tool{
		Name:         toolName,
		Title:        CamelToTitle(methodName),
		Description:  buildDescription(method),
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Annotations:  annotations,
	}

	s.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var inputJSON json.RawMessage
		if req.Params.Arguments != nil {
			var err error
			inputJSON, err = json.Marshal(req.Params.Arguments)
			if err != nil {
				return errorResult("invalid input: " + err.Error()), nil
			}
		}

		respJSON, err := caller.Call(ctx, method, inputJSON)
		if err != nil {
			slog.Error("tool call failed", "tool", toolName, "method", string(method.FullName()), "error", err)
			return errorResult(err.Error()), nil
		}

		var structured map[string]any
		if err := json.Unmarshal(respJSON, &structured); err != nil {
			return errorResult("invalid response JSON: " + err.Error()), nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(respJSON)},
			},
			StructuredContent: structured,
		}, nil
	})
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
		IsError: true,
	}
}

// buildDescription generates a human-readable description for an RPC method.
func buildDescription(method protoreflect.MethodDescriptor) string {
	methodName := string(method.Name())

	// Extract the resource name from the method name
	var action, resource string
	for _, prefix := range []string{"Create", "Get", "List", "Update", "Delete"} {
		if strings.HasPrefix(methodName, prefix) {
			action = prefix
			resource = methodName[len(prefix):]
			break
		}
	}

	if action != "" && resource != "" {
		readable := CamelToWords(resource)
		if action == "List" {
			return fmt.Sprintf("List %s.", readable)
		}
		return fmt.Sprintf("%s a %s.", action, readable)
	}

	return CamelToWords(methodName) + "."
}

// CamelToSnake converts CamelCase to snake_case.
func CamelToSnake(s string) string { return camelSplit(s, '_', true) }

// CamelToTitle converts CamelCase to space-separated title case words.
func CamelToTitle(s string) string { return camelSplit(s, ' ', false) }

// CamelToWords converts CamelCase to space-separated lowercase words.
func CamelToWords(s string) string { return camelSplit(s, ' ', true) }

func camelSplit(s string, sep byte, toLower bool) string {
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			result.WriteByte(sep)
		}
		if toLower {
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
