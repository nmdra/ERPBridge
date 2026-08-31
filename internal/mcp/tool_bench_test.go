package mcp

import (
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/nmdra/ERPBridge/internal/connector"
)

var (
	benchmarkEndpoint connector.EndpointConfig
	benchmarkQuery    url.Values
	benchmarkBody     io.Reader
)

func BenchmarkPrepareERPCall(b *testing.B) {
	cases := []struct {
		name string
		tool *Tool
		args map[string]any
	}{
		{
			name: "legacy_get_query",
			tool: &Tool{Metadata: Metadata{Name: "list_employees"}, Spec: ToolSpec{
				Execution: Execution{Method: http.MethodGet, Endpoint: "https://erp.example/employees"},
			}},
			args: map[string]any{"department": "Operations", "includeInactive": false, "limit": 25},
		},
		{
			name: "generated_locations",
			tool: &Tool{Metadata: Metadata{Name: "update_employee"}, Spec: ToolSpec{
				Execution: Execution{
					Method:             http.MethodPost,
					Endpoint:           "https://erp.example/employees/{id}",
					Mapping:            map[string]string{"employeeID": "id", "search": "q", "trace": "X-Trace"},
					ParameterLocations: map[string]string{"employeeID": "path", "search": "query", "trace": "header", "details": "body"},
				},
			}},
			args: map[string]any{
				"employeeID": "E/0001",
				"search":     "open items",
				"trace":      "trace-1",
				"details":    map[string]any{"active": true, "department": "Operations"},
			},
		},
		{
			name: "complete_body",
			tool: &Tool{Metadata: Metadata{Name: "bulk_update"}, Spec: ToolSpec{
				Execution: Execution{
					Method:       http.MethodPost,
					Endpoint:     "https://erp.example/employees/bulk",
					BodyArgument: "body",
					ParameterLocations: map[string]string{
						"body": "body",
					},
				},
			}},
			args: map[string]any{"body": []any{"E-0001", "E-0002", "E-0003"}},
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var err error
				benchmarkEndpoint, benchmarkQuery, benchmarkBody, err = tc.tool.prepareERPCall(tc.args)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
