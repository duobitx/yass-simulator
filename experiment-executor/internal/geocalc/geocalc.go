package geocalc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/duobitx/yass-internal-components/go-common/common_slog"
	"github.com/m-szalik/goutils"
	errors2 "github.com/pkg/errors"
)

const shmFilePath = "/dev/shm/geo_calc_shared_memory"

func run(ctx context.Context, name string, args ...string) error {
	llog := common_slog.FromContext(ctx)
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("getting stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("getting stderr pipe: %w", err)
	}

	llog.Info("Starting command", "name", name, "args", args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting command %s: %w", name, err)
	}
	pid := cmd.Process.Pid
	llog.Info(fmt.Sprintf("Process %s %v started", name, args), "pid", pid)
	go func() { // kill gocalc process on context cancel
		<-ctx.Done()
		llog.Info("closing geo_calc due to", "ctxError", ctx.Err())
		proc, err := os.FindProcess(pid)
		if err != nil {
			llog.Warn("Error finding process", "error", err)
			return
		}
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			llog.Warn("Error sending signal:", "error", err)
			return
		}
	}()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			llog.Warn(fmt.Sprintf("[geocalc stderr] %s", scanner.Text()))
		}
		if err := scanner.Err(); err != nil {
			slog.Default().Error("[geocalc] error reading stderr", "error", err)
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			llog.Info(fmt.Sprintf("[geocalc stdout] %s", scanner.Text()))
		}
		if err := scanner.Err(); err != nil {
			slog.Default().Error("[geocalc] error reading stdout", "error", err)
		}
	}()

	err = cmd.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("command %q failed: exit code %d", name, exitErr.ExitCode())
		}
		return fmt.Errorf("command %q failed: %w", name, err)
	}
	wg.Wait()
	return nil
}

func readFromGeoCalcBlocking(ctx context.Context, tickTime time.Duration, chOut chan<- *GeoCalcUpdate) error {
	shmFile, err := os.Open(shmFilePath)
	if err != nil {
		return fmt.Errorf("cannot open %s:: %w ", shmFilePath, err)
	}
	defer goutils.CloseQuietly(shmFile)
	data, err := syscall.Mmap(
		int(shmFile.Fd()),
		0,
		int(unsafe.Sizeof(common{})),
		syscall.PROT_READ,
		syscall.MAP_SHARED,
	)
	if err != nil {
		return fmt.Errorf("mmap error:: %w", err)
	}
	defer func() { _ = syscall.Munmap(data) }()
	commonMem := (*common)(unsafe.Pointer(&data[0]))

	ticker := time.NewTicker(tickTime)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			timeout := time.Now().Add(200 * time.Millisecond)
		busyLoop:
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					if commonMem.Busy > 0 {
						if time.Now().After(timeout) {
							slog.Default().Error("cannot convert geoUpdate as it is still busy")
							break busyLoop
						}
						time.Sleep(2 * time.Millisecond)
						continue
					}
					update, err := Convert(commonMem)
					if err != nil {
						slog.Default().Error("cannot convert geoCalc response to geoUpdate", "error", err)
					} else {
						pos := ""
						for _, elem := range update.FsNodeInfos {
							pos = fmt.Sprintf("%s   %v", pos, elem)
						}
						slog.Default().Info("geo update", "content", strings.TrimSpace(pos))
						chOut <- update
					}
					break busyLoop
				}
			}
		}
	}
}

func RunGeoCalc(parentCctx context.Context, interval time.Duration) (<-chan *GeoCalcUpdate, <-chan error) {
	chOut := make(chan *GeoCalcUpdate)
	chErr := make(chan error, 1)
	llog := slog.Default().WithGroup("geocalc")
	ctx := common_slog.NewContext(parentCctx, llog)
	var wg sync.WaitGroup
	wg.Add(2) // One for process runner, one for file waiter/reader
	llog.Info("Starting geo_calc process")
	// Start the geo_calc process
	go func() {
		defer wg.Done()
		llog.Info("Starting geo_calc process")
		err := run(ctx, "stdbuf", "-oL", "-eL", "./geo_calc", goutils.Env("EXPERIMENT_JSON_FILE_PATH", "/mnt/shared/experiment.json"))
		if err != nil {
			select {
			case chErr <- err:
				llog.Error("error running geo_calc", "error", err)
			case <-ctx.Done():
				llog.Info("context canceled, exiting")
			}
		}
	}()

	// Wait for shared memory file and start reader
	go func() {
		defer wg.Done()
		fileCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		err := WaitForFile(fileCtx, shmFilePath)
		if err != nil {
			select {
			case chErr <- errors2.Wrapf(err, "waiting for file %s", shmFilePath):
			case <-ctx.Done():
			}
			return
		}

		slog.Default().Info("Reading form geo_calc started")
		err = readFromGeoCalcBlocking(ctx, interval, chOut)
		if err != nil {
			select {
			case chErr <- errors2.Wrap(err, "readFromGeoCalc"):
			case <-ctx.Done():
			}
		}
	}()

	// Close channels after all goroutines finish
	go func() {
		wg.Wait()
		close(chErr)
		close(chOut)
	}()

	return chOut, chErr
}
