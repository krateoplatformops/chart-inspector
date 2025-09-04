package tools

import (
	"testing"

	sigsyaml "sigs.k8s.io/yaml"
)

func TestAddOrUpdateFieldInValues(t *testing.T) {
	tests := []struct {
		name        string
		values      string
		value       interface{}
		fields      []string
		expected    string
		expectError bool
	}{
		{
			name:     "add single field to empty values",
			values:   "{}",
			value:    "test-value",
			fields:   []string{"testField"},
			expected: "testField: test-value\n",
		},
		{
			name:     "add nested field to empty values",
			values:   "{}",
			value:    "test-value",
			fields:   []string{"global", "testField"},
			expected: "global:\n  testField: test-value\n",
		},
		{
			name:     "update existing field",
			values:   "testField: old-value",
			value:    "new-value",
			fields:   []string{"testField"},
			expected: "testField: new-value\n",
		},
		{
			name:     "add to existing nested structure",
			values:   "global:\n  existing: value",
			value:    "test-value",
			fields:   []string{"global", "newField"},
			expected: "global:\n  existing: value\n  newField: test-value\n",
		},
		{
			name:     "deeply nested field",
			values:   "{}",
			value:    "test-value",
			fields:   []string{"level1", "level2", "level3", "testField"},
			expected: "level1:\n  level2:\n    level3:\n      testField: test-value\n",
		},
		{
			name:        "field exists but is not a map",
			values:      "testField: string-value",
			value:       "test-value",
			fields:      []string{"testField", "nested"},
			expectError: true,
		},
		{
			name:        "invalid yaml",
			values:      "invalid: yaml: content:",
			value:       "test-value",
			fields:      []string{"testField"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AddOrUpdateFieldInValues([]byte(tt.values), tt.value, tt.fields...)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if string(result) != tt.expected {
				t.Errorf("expected:\n%s\ngot:\n%s", tt.expected, string(result))
			}
		})
	}
}

func TestInjectValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		opts     CompositionValues
		expected map[string]interface{}
	}{
		{
			name:  "inject all values to empty yaml",
			input: "{}",
			opts: CompositionValues{
				KrateoNamespace:      "krateo-system",
				CompositionName:      "test-composition",
				CompositionNamespace: "default",
				CompositionId:        "comp-123",
				CompositionGroup:     "test.group",
				CompositionResource:  "testresources",
				CompositionKind:      "TestResource",
				GracefullyPaused:     true,
			},
			expected: map[string]interface{}{
				"global": map[string]interface{}{
					"gracefullyPaused":            "true",
					"compositionNamespace":        "default",
					"compositionName":             "test-composition",
					"krateoNamespace":             "krateo-system",
					"compositionId":               "comp-123",
					"compositionApiVersion":       "",
					"compositionGroup":            "test.group",
					"compositionInstalledVersion": "",
					"compositionResource":         "testresources",
					"compositionKind":             "TestResource",
				},
			},
		},
		{
			name:  "inject values without gracefully paused",
			input: "{}",
			opts: CompositionValues{
				KrateoNamespace:      "krateo-system",
				CompositionName:      "test-composition",
				CompositionNamespace: "default",
				CompositionId:        "comp-123",
				CompositionGroup:     "test.group",
				CompositionResource:  "testresources",
				CompositionKind:      "TestResource",
				GracefullyPaused:     false,
			},
			expected: map[string]interface{}{
				"global": map[string]interface{}{
					"gracefullyPaused":            "false",
					"compositionNamespace":        "default",
					"compositionName":             "test-composition",
					"krateoNamespace":             "krateo-system",
					"compositionId":               "comp-123",
					"compositionApiVersion":       "",
					"compositionGroup":            "test.group",
					"compositionInstalledVersion": "",
					"compositionResource":         "testresources",
					"compositionKind":             "TestResource",
				},
			},
		},
		{
			name:  "inject values to existing yaml",
			input: "existing:\n  field: value\nglobal:\n  existingGlobal: value",
			opts: CompositionValues{
				KrateoNamespace:      "krateo-system",
				CompositionName:      "test-composition",
				CompositionNamespace: "default",
				CompositionId:        "comp-123",
				CompositionGroup:     "test.group",
				CompositionResource:  "testresources",
				CompositionKind:      "TestResource",
			},
			expected: map[string]interface{}{
				"existing": map[string]interface{}{
					"field": "value",
				},
				"global": map[string]interface{}{
					"gracefullyPaused":            "false",
					"existingGlobal":              "value",
					"compositionNamespace":        "default",
					"compositionName":             "test-composition",
					"krateoNamespace":             "krateo-system",
					"compositionId":               "comp-123",
					"compositionApiVersion":       "",
					"compositionGroup":            "test.group",
					"compositionInstalledVersion": "",
					"compositionResource":         "testresources",
					"compositionKind":             "TestResource",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := InjectValues([]byte(tt.input), tt.opts)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			var resultMap map[string]interface{}
			if err := sigsyaml.Unmarshal(result, &resultMap); err != nil {
				t.Errorf("failed to unmarshal result: %v", err)
				return
			}

			if !deepEqual(resultMap, tt.expected) {
				t.Errorf("expected:\n%+v\ngot:\n%+v", tt.expected, resultMap)
			}
		})
	}
}

func TestInjectValuesError(t *testing.T) {
	invalidYaml := "invalid: yaml: content:"
	opts := CompositionValues{
		KrateoNamespace: "krateo-system",
	}

	_, err := InjectValues([]byte(invalidYaml), opts)
	if err == nil {
		t.Errorf("expected error for invalid yaml but got none")
	}
}

// Helper function to compare maps deeply
func deepEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}

	for key, valueA := range a {
		valueB, exists := b[key]
		if !exists {
			return false
		}

		switch va := valueA.(type) {
		case map[string]interface{}:
			if vb, ok := valueB.(map[string]interface{}); ok {
				if !deepEqual(va, vb) {
					return false
				}
			} else {
				return false
			}
		default:
			if valueA != valueB {
				return false
			}
		}
	}

	return true
}
