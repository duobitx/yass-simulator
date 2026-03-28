package experiment

import (
	"testing"

	yassv1 "github.com/duobitx/yass-operator/api/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProcessTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		values   map[string]any
		expected string
		wantErr  bool
	}{
		{
			name:     "simple substitution",
			template: "Hello {{ .name }}!",
			values:   map[string]any{"name": "World"},
			expected: "Hello World!",
		},
		{
			name:     "experiment object access",
			template: "Experiment: {{ .experiment.Name }} in {{ .namespace }}",
			values: map[string]any{
				"experiment": &yassv1.Experiment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-exp",
					},
				},
				"namespace": "test-ns",
			},
			expected: "Experiment: test-exp in test-ns",
		},
		{
			name:     "invalid template",
			template: "Hello {{ .name }",
			values:   map[string]any{"name": "World"},
			wantErr:  true,
		},
		{
			name:     "missing key (should result in <no value> or empty depending on how we handle it)",
			template: "Hello {{ .missing }}!",
			values:   map[string]any{"name": "World"},
			expected: "Hello <no value>!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processTemplate([]byte(tt.template), tt.values)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(got))
			}
		})
	}
}
