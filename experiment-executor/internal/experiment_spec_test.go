package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	t.Setenv("EXPERIMENT_FILE", "../experiment.json")
	_, err := LoadExperimentJson()
	assert.NoError(t, err)
}
