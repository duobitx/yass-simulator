package geocalc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	geocalcproto "github.com/duobitx/yass-internal-components/go-common/proto/go"
)

func TestGeoCalcSharedMemoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	geoCalcBin := os.Getenv("GEOCALC_BIN")
	if geoCalcBin == "" {
		geoCalcBin = filepath.Clean("../../../geo-calculator/V6/geo_calc")
	}
	geoCalcJSON := os.Getenv("GEOCALC_JSON")
	if geoCalcJSON == "" {
		geoCalcJSON = filepath.Clean("../../../geo-calculator/V6/examp_marekg.json")
	}

	if _, err := os.Stat(geoCalcBin); err != nil {
		t.Skipf("geo_calc binary not found at %s (set GEOCALC_BIN to run): %v", geoCalcBin, err)
	}
	if _, err := os.Stat(geoCalcJSON); err != nil {
		t.Fatalf("json input not found at %s: %v", geoCalcJSON, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, geoCalcBin, geoCalcJSON)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting geo_calc: %v", err)
	}

	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGINT)
		}
		waitDone := make(chan error, 1)
		go func() {
			waitDone <- cmd.Wait()
		}()
		select {
		case <-time.After(3 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-waitDone
		case <-waitDone:
		}
	}()

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	if err := WaitForFile(waitCtx, shmFilePath); err != nil {
		t.Fatalf("waiting for shared memory file: %v", err)
	}

	mem, cleanup, err := mmapSharedMemory(shmFilePath)
	if err != nil {
		t.Fatalf("mmap shared memory: %v", err)
	}
	defer cleanup()

	var msg *geocalcproto.GeoCommon
	readDeadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(readDeadline) {
		msg, err = readGeoCalcMessage(mem)
		if err == nil {
			break
		}
		if !errors.Is(err, errSharedMemBusy) {
			t.Fatalf("reading shared memory frame: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("did not read any valid frame before timeout: %v", err)
	}

	if msg.GetNsat() <= 0 {
		t.Fatalf("invalid nsat value: %d", msg.GetNsat())
	}
	if len(msg.GetItems()) == 0 {
		t.Fatalf("no items in decoded protobuf frame")
	}
	t.Logf(
		"decoded protobuf frame: nsat=%d nbs=%d items=%d distances=%d",
		msg.GetNsat(), msg.GetNbs(), len(msg.GetItems()), len(msg.GetDistances()),
	)
	for _, item := range msg.GetItems() {
		t.Logf("  item: id=%d name=%s lat=%f lon=%f alt=%f, in_sun=%t", item.GetId(), item.GetName(), item.GetLat(), item.GetLon(), item.GetAlt(), item.GetInTheSun())
	}
	for _, d := range msg.GetDistances() {
		t.Logf("  distance: between_id=(%d;%d) los=%t, distance=%f", d.ItemIdA, d.ItemIdB, d.Los, d.Distance)
	}
}

func mmapSharedMemory(path string) ([]byte, func(), error) {
	shmFile, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}

	fi, err := shmFile.Stat()
	if err != nil {
		_ = shmFile.Close()
		return nil, nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if fi.Size() < shmHeaderSize {
		_ = shmFile.Close()
		return nil, nil, fmt.Errorf("shared memory too small: %d", fi.Size())
	}

	mem, err := syscall.Mmap(int(shmFile.Fd()), 0, int(fi.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		_ = shmFile.Close()
		return nil, nil, fmt.Errorf("mmap %s: %w", path, err)
	}

	cleanup := func() {
		_ = syscall.Munmap(mem)
		_ = shmFile.Close()
	}
	return mem, cleanup, nil
}
