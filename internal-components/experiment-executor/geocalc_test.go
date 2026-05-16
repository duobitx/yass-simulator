package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/experiment-executor/internal/geocalc"
)

// TestRunGeoCalc is a manual integration helper. It needs the geo_calc binary
// in the current working directory (or pointed to via GEOCALC_BIN) and is
// skipped in -short mode or when the binary is unavailable so that
// `go test ./...` does not block for the full timeout.
func TestRunGeoCalc(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	geoCalcBin := os.Getenv("GEOCALC_BIN")
	if geoCalcBin == "" {
		geoCalcBin = filepath.Clean("./geo_calc")
	}
	if _, err := os.Stat(geoCalcBin); err != nil {
		t.Skipf("geo_calc binary not found at %s (set GEOCALC_BIN to run): %v", geoCalcBin, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dataCh, errCh := geocalc.RunGeoCalc(ctx, 2*time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case geoUpdate := <-dataCh:
			if geoUpdate != nil {
				fmt.Printf("%s UPDATE: %+v\n", time.Now(), *geoUpdate)
			}
		case err := <-errCh:
			if err != nil {
				fmt.Println("ERROR", err)
			}
		}
	}
}
