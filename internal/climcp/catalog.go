package climcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// descriptor is one fixed management tool. Registration and dispatch share
// this record, so an advertised tool always has a matching fixed operation and
// permission requirement.
type descriptor struct {
	// requiredGroup is the startup write group needed to advertise and call
	// this tool. An empty group means inspection or local-only work.
	requiredGroup WriteGroup
	// tool is the advertised schema.
	tool mcp.Tool
	// handler builds the handler for this session.
	handler func(s *Server) server.ToolHandlerFunc
}

// registerCatalogue registers every permitted tool from the fixed descriptor
// table. The table is the single source of truth for registration and
// dispatch, so a tool cannot be advertised without a matching descriptor.
//
// The initial work package establishes the seam only; the read and write
// descriptors are added by their own work packages.
func registerCatalogue(s *Server) {
	for _, entry := range catalogue {
		if entry.requiredGroup != "" && !s.Allows(entry.requiredGroup) {
			continue
		}
		s.register(entry.tool, entry.handler(s))
	}
}

// catalogue is the complete fixed tool list. Adding an entry requires a plan
// amendment: no generic command, shell, path, or URL tool may be added here.
var catalogue []descriptor
