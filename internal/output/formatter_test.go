package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

type MockData struct {
	Name string `json:"name" yaml:"name"`
}

func (m *MockData) RenderTable(w io.Writer) error {
	tw := NewTabWriter(w)
	if _, err := fmt.Fprintf(tw, "NAME\n%s\n", m.Name); err != nil {
		return err
	}
	return tw.Flush()
}

type NonTableData struct {
	ID string `json:"id"`
}

func TestFormatter_Print(t *testing.T) {
	data := &MockData{Name: testName}

	t.Run("JSON", func(t *testing.T) {
		buf := &bytes.Buffer{}
		f := &Formatter{Format: FormatJSON, Out: buf}
		if err := f.Print(data); err != nil {
			t.Fatal(err)
		}
		expected := "{\n  \"name\": \"" + testName + "\"\n}\n"
		if buf.String() != expected {
			t.Errorf("expected %q, got %q", expected, buf.String())
		}
	})

	t.Run("YAML", func(t *testing.T) {
		buf := &bytes.Buffer{}
		f := &Formatter{Format: FormatYAML, Out: buf}
		if err := f.Print(data); err != nil {
			t.Fatal(err)
		}
		expected := "name: " + testName + "\n"
		if buf.String() != expected {
			t.Errorf("expected %q, got %q", expected, buf.String())
		}
	})

	t.Run("Table", func(t *testing.T) {
		buf := &bytes.Buffer{}
		f := &Formatter{Format: FormatTable, Out: buf}
		if err := f.Print(data); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "NAME") || !strings.Contains(buf.String(), testName) {
			t.Errorf("unexpected table output: %q", buf.String())
		}
	})

	t.Run("Table Fallback", func(t *testing.T) {
		buf := &bytes.Buffer{}
		f := &Formatter{Format: FormatTable, Out: buf}
		nonTableData := &NonTableData{ID: "123"}
		if err := f.Print(nonTableData); err != nil {
			t.Fatal(err)
		}
		expected := "{\"id\":\"123\"}\n"
		if buf.String() != expected {
			t.Errorf("expected %q, got %q", expected, buf.String())
		}
	})
}

func TestRawResponse(t *testing.T) {
	t.Run("RenderTable Success", func(t *testing.T) {
		jsonData := testNameJSON
		target := &MockData{}
		rr := NewRawResponse(strings.NewReader(jsonData), target)
		buf := &bytes.Buffer{}
		if err := rr.RenderTable(buf); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "NAME") || !strings.Contains(buf.String(), testName) {
			t.Errorf("unexpected table output: %q", buf.String())
		}
	})

	t.Run("RenderTable Fallback", func(t *testing.T) {
		jsonData := `{"id": "123"}`
		target := &NonTableData{}
		rr := NewRawResponse(strings.NewReader(jsonData), target)
		buf := &bytes.Buffer{}
		if err := rr.RenderTable(buf); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "123") {
			t.Errorf("unexpected output: %q", buf.String())
		}
	})

	t.Run("RenderTable Decode Error", func(t *testing.T) {
		invalidJSON := testInvalidJSON
		target := &MockData{}
		rr := NewRawResponse(strings.NewReader(invalidJSON), target)
		buf := &bytes.Buffer{}
		if err := rr.RenderTable(buf); err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("MarshalJSON Success", func(t *testing.T) {
		jsonData := testNameJSON
		target := &MockData{}
		rr := NewRawResponse(strings.NewReader(jsonData), target)
		b, err := rr.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		var decoded MockData
		if err := json.Unmarshal(b, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Name != testName {
			t.Errorf("expected %s, got %s", testName, decoded.Name)
		}
	})

	t.Run("MarshalJSON Decode Error", func(t *testing.T) {
		invalidJSON := testInvalidJSON
		target := &MockData{}
		rr := NewRawResponse(strings.NewReader(invalidJSON), target)
		if _, err := rr.MarshalJSON(); err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("MarshalYAML Success", func(t *testing.T) {
		jsonData := testNameJSON
		target := &MockData{}
		rr := NewRawResponse(strings.NewReader(jsonData), target)
		y, err := rr.MarshalYAML()
		if err != nil {
			t.Fatal(err)
		}
		data, ok := y.(*MockData)
		if !ok {
			t.Fatalf("expected *MockData, got %T", y)
		}
		if data.Name != testName {
			t.Errorf("expected %s, got %s", testName, data.Name)
		}
	})

	t.Run("MarshalYAML Decode Error", func(t *testing.T) {
		invalidJSON := testInvalidJSON
		target := &MockData{}
		rr := NewRawResponse(strings.NewReader(invalidJSON), target)
		if _, err := rr.MarshalYAML(); err == nil {
			t.Error("expected error, got nil")
		}
	})
}

const (
	testNameJSON    = `{"name": "Test"}`
	testInvalidJSON = `{invalid}`
	testName        = "Test"
)

type ErrorWriter struct {
	Called bool
}

func (e *ErrorWriter) Write(_ []byte) (n int, err error) {
	e.Called = true
	return 0, errors.New("write error")
}

func TestFormatter_Print_Errors(t *testing.T) {
	data := &MockData{Name: "Test"}

	t.Run("JSON Write Error", func(t *testing.T) {
		ew := &ErrorWriter{}
		f := &Formatter{Format: FormatJSON, Out: ew}
		if err := f.Print(data); err == nil {
			t.Error("expected error, got nil")
		}
		if !ew.Called {
			t.Error("expected Write to be called")
		}
	})
}

func TestNewTabWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	tw := NewTabWriter(buf)
	if tw == nil {
		t.Fatal("expected non-nil tabwriter")
	}
	_, _ = fmt.Fprintln(tw, "A\tB")
	_ = tw.Flush()
	if !strings.Contains(buf.String(), "A") {
		t.Errorf("expected output to contain A, got %q", buf.String())
	}
}

func TestNonNilConvertsANilSliceToACollection(t *testing.T) {
	var nothing []string
	got := NonNil(nothing)
	if got == nil {
		t.Fatal("NonNil returned nil, so the rendered collection would be null")
	}
	if len(got) != 0 {
		t.Fatalf("NonNil returned %d entries, want none", len(got))
	}

	present := []string{"one"}
	if got := NonNil(present); len(got) != 1 || got[0] != "one" {
		t.Fatalf("NonNil changed a non-empty slice: %v", got)
	}
}
