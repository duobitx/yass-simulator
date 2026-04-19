package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/duobitx/yass-internal-components/experiment-executor/internal/geocalc"
)

func TestRunGeoCalc(_ *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer func() {
		cancel()
	}()
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
