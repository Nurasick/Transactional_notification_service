package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender(t *testing.T) {

	tests := []struct {
		name     string
		template string
		data     map[string]any
		want     string
	}{
		{
			name:     "fills all fields",
			template: "Hello {{.UserName}}, order {{.OrderID}}",
			data:     map[string]any{"UserName": "Test", "OrderID": "ORD-42"},
			want:     "Hello Test, order ORD-42",
		},
		{
			name:     "missing key renders no value",
			template: "Order {{.OrderID}}",
			data:     map[string]any{},
			want:     "Order <no value>",
		},
		{
			name:     "keys are case sensitive",
			template: "Hello {{.UserName}}",
			data:     map[string]any{"Username": "Test"},
			want:     "Hello <no value>",
		},
		{
			name:     "extra keys are ignored",
			template: "Hello {{.UserName}}",
			data:     map[string]any{"UserName": "Test", "OrderID": "ORD-42"},
			want:     "Hello Test",
		},
		{
			name:     "nil data",
			template: "Hello {{.UserName}}",
			data:     nil,
			want:     "Hello <no value>",
		},
		{
			name:     "nested field",
			template: "Hello {{.User.Name}}",
			data:     map[string]any{"User": map[string]any{"Name": "Test"}},
			want:     "Hello Test",
		},
		{
			name:     "html is not escaped",
			template: "{{.Note}}",
			data:     map[string]any{"Note": "<script>"},
			want:     "<script>",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := render([]byte(test.template), "test.html", test.data)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)

		})
	}

}

func TestRender_Errors(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     map[string]any
		wantErr  string
	}{
		{
			name:     "unclosed action fails to parse",
			template: "Hi {{.Name",
			wantErr:  "parse template broken.html",
		},
		{
			name:     "field on a string fails to execute",
			template: "{{.Name.Foo}}",
			data:     map[string]any{"Name": "Nurali"},
			wantErr:  "render template broken.html",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := render([]byte(test.template), "broken.html", test.data)

			require.Error(t, err)
			assert.ErrorContains(t, err, test.wantErr)
			assert.Empty(t, got)
		})
	}
}

func TestRender_LargeJSONNumber(t *testing.T) {
	var data map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"Amount":4500000}`), &data))
	got, err := render([]byte("Amount:{{.Amount}}"), "test.html", data)

	require.NoError(t, err)
	assert.Equal(t, "Amount:4.5e+06", got)
}

func TestShippedTemplates(t *testing.T) {
	examplePayloads := map[string]map[string]any{
		"order.placed.html":     {"UserName": "Nurali", "OrderID": "ORD-42", "Amount": "4500"},
		"payment.received.html": {"UserName": "Nurali", "Amount": "4500", "TransactionID": "TX-777"},
	}

	const dir = "../../templates"

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".html" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			payload, ok := examplePayloads[e.Name()]
			require.True(t, ok, "add an example payload for %s", e.Name())
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			require.NoError(t, err)

			got, err := render(raw, e.Name(), payload)

			require.NoError(t, err)
			assert.NotContains(t, got, "<no value>")
		})
	}

}
