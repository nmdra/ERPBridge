// Package output provides unified formatting (table, JSON, YAML) for CLI command responses.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/goccy/go-yaml"
)

// Format represents the output format type (table, json, yaml).
type Format string

const (
	// FormatTable renders output as an aligned plain-text table.
	FormatTable Format = "table"
	// FormatJSON renders output as indented JSON.
	FormatJSON Format = "json"
	// FormatYAML renders output as YAML.
	FormatYAML Format = "yaml"
)

// Formatter handles serializing data structures into the requested output format.
type Formatter struct {
	Format Format
	Out    io.Writer // defaults to os.Stdout
}

// Print serialises v in the configured format.
// For table format, v must implement TableRenderer.
func (f *Formatter) Print(v any) error {
	switch f.Format {
	case FormatJSON:
		enc := json.NewEncoder(f.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)

	case FormatYAML:
		return yaml.NewEncoder(f.Out).Encode(v)

	default: // table
		if tr, ok := v.(TableRenderer); ok {
			return tr.RenderTable(f.Out)
		}
		// Fallback: JSON for types that don't implement table rendering
		return json.NewEncoder(f.Out).Encode(v)
	}
}

// TableRenderer is implemented by any type that knows how to
// render itself as an aligned table.
type TableRenderer interface {
	RenderTable(w io.Writer) error
}

// RawResponse wraps an io.Reader (like an HTTP response body)
// and handles decoding it into a target struct for formatting.
type RawResponse struct {
	reader io.Reader
	target any
}

// NewRawResponse constructs a RawResponse wrapping an io.Reader and a target decode struct.
func NewRawResponse(r io.Reader, target any) *RawResponse {
	return &RawResponse{reader: r, target: target}
}

// RenderTable decodes the payload and renders it as an aligned table.
func (r *RawResponse) RenderTable(w io.Writer) error {
	if err := json.NewDecoder(r.reader).Decode(r.target); err != nil {
		return err
	}
	if tr, ok := r.target.(TableRenderer); ok {
		return tr.RenderTable(w)
	}
	// Fallback to JSON if target doesn't implement TableRenderer
	b, _ := json.MarshalIndent(r.target, "", "  ")
	_, err := fmt.Fprintln(w, string(b))
	return err
}

// MarshalJSON returns the JSON encoding of the wrapped response.
func (r *RawResponse) MarshalJSON() ([]byte, error) {
	if err := json.NewDecoder(r.reader).Decode(r.target); err != nil {
		return nil, err
	}
	return json.Marshal(r.target)
}

// MarshalYAML returns the YAML representation of the wrapped response.
func (r *RawResponse) MarshalYAML() (any, error) {
	if err := json.NewDecoder(r.reader).Decode(r.target); err != nil {
		return nil, err
	}
	return r.target, nil
}

// NonNil returns an empty slice when items is nil, so a rendered collection is
// always a list rather than null. A consumer that reads the output as data
// cannot tell an absent collection from an empty one otherwise.
func NonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

// NewTabWriter returns a pre-configured tab writer for table output.
func NewTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
}
