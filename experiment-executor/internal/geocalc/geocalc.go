package geocalc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/m-szalik/goutils"
	errors2 "github.com/pkg/errors"
)

const shmFilePath = "/dev/shm/geo_calc_shared_memory"

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("getting stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("getting stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting command %s: %w", name, err)
	}
	pid := cmd.Process.Pid
	slog.Default().Info(fmt.Sprintf("Process %s %v started", name, args), "pid", pid)
	go func() {
		<-ctx.Done()
		slog.Default().Info("closing geo_calc due to", "ctxError", ctx.Err())
		proc, err := os.FindProcess(pid)
		if err != nil {
			slog.Default().Warn("Error finding process", "error", err)
			return
		}
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			slog.Default().Warn("Error sending signal:", "error", err)
			return
		}
	}()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			slog.Default().Warn(fmt.Sprintf("[geocalc] %s", scanner.Text()))
		}
		if err := scanner.Err(); err != nil {
			slog.Default().Error("[geocalc] error reading stderr", "error", err)
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			slog.Default().Info(fmt.Sprintf("[geocalc] %s", scanner.Text()))
		}
		if err := scanner.Err(); err != nil {
			slog.Default().Error("[geocalc] error reading stdout", "error", err)
		}
	}()

	err = cmd.Wait()
	wg.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("command %q failed: exit code %d", name, exitErr.ExitCode())
		}
		return fmt.Errorf("command %q failed: %w", name, err)
	}
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
						chOut <- update
					}
					break busyLoop
				}
			}
		}
	}
}

func RunGeoCalc(ctx context.Context, interval time.Duration) (<-chan *GeoCalcUpdate, <-chan error) {
	chOut := make(chan *GeoCalcUpdate)
	chErr := make(chan error, 1)
	go func() {
		//err := run(ctx, "stdbuf", "-oL", "-eL", "./geo_calc", "./experiment.json")
		err := run(ctx, "./geo_calc", "./experiment.json")
		if err != nil {
			chErr <- err
		}
		close(chErr)
		close(chOut)
	}()
	fileCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer func() {
		cancel()
	}()
	err := WaitForFile(fileCtx, shmFilePath)
	if err != nil {
		chErr <- errors2.Wrapf(err, "waiting for file %s", shmFilePath)
	} else {
		go func() {
			slog.Default().Info("Reading form geo_calc started")
			err := readFromGeoCalcBlocking(ctx, interval, chOut)
			if err != nil {
				chErr <- errors2.Wrap(err, "readFromGeoCalc")
			}
		}()
	}
	return chOut, chErr
}
