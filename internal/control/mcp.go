package control

import (
	"context"
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServeMCP serves the commands as MCP tools over stdio, each forwarded to
// the running app.
func ServeMCP(ctx context.Context, version string) error {
	s := mcp.NewServer(&mcp.Implementation{Name: "lilt", Title: "Lilt (QQ Music)", Version: version}, &mcp.ServerOptions{
		Instructions: "Controls the Lilt QQ Music desktop app, which must be running. " +
			"Find songs with search, then use their numeric `id` with play, like or playlist_add. " +
			"playlists lists the user's playlists; only created ones (kind=created) can be renamed or edited.",
	})
	for _, c := range Commands {
		s.AddTool(&mcp.Tool{Name: c.Name, Description: c.Help, InputSchema: schema(c)}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			data, err := Call(ctx, c.Name, req.Params.Arguments)
			if err != nil {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}, StructuredContent: json.RawMessage(data)}, nil
		})
	}
	return s.Run(ctx, &mcp.StdioTransport{})
}

func schema(c Command) *jsonschema.Schema {
	s := &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{}}
	for _, p := range c.Params {
		ps := &jsonschema.Schema{Type: p.Type, Description: p.Help}
		if p.Type == "integers" {
			ps.Type, ps.Items = "array", &jsonschema.Schema{Type: "integer"}
		}
		for _, e := range p.Enum {
			ps.Enum = append(ps.Enum, e)
		}
		s.Properties[p.Name] = ps
		if p.Required {
			s.Required = append(s.Required, p.Name)
		}
	}
	return s
}
