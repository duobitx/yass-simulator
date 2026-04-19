package internal

import (
	"encoding/json"
	"os"

	"github.com/duobitx/yass-simulator/internal-components/go-common/cmodel"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
)

func LoadExperimentJson() (*cmodel.ExperimentDefinition, error) {
	experimentJson := goutils.Env("EXPERIMENT_FILE", "experiment.json")
	exp := &cmodel.ExperimentDefinition{}
	buff, err := os.ReadFile(experimentJson)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot load file %s", experimentJson)
	}
	err = json.Unmarshal(buff, exp)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal experiment json")
	}
	return exp, nil
}
