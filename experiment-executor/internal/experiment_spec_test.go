package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	_, err := LoadExperimentJson()
	assert.NoError(t, err)
}
