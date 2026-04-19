package hw

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	"github.com/m-szalik/goutils"
)

func Read() (*yassv1.HardwareSpec, error) {
	hwFile := path.Join(goutils.Env("SHARED_VOLUME_PATH", "."), "hardware.json")
	buff, err := os.ReadFile(hwFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read hardware file: %w", err)
	}
	hw := &yassv1.HardwareSpec{}
	if err := json.Unmarshal(buff, hw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hardware file: %w", err)
	}
	return hw, nil
}
