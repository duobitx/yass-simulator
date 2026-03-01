package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	absExperimentPath, _ := os.Getwd()
	absExperimentPath += "/experiment.json"
	absExperimentPath, err := filepath.Abs(absExperimentPath)
	assert.NoError(t, err)

	t.Setenv("EXPERIMENT_FILE", "./experiment.json")
	t.Setenv("EXPERIMENT_JSON_FILE_PATH", absExperimentPath)
	_, err = LoadExperimentJson()
	assert.NoError(t, err)
}
