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

type Common struct {
	Busy    int32
	Nsat    int32
	Nbs     int32
	UtcDttm [32]byte
}

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

func readFromGeoCalcBlocking(ctx context.Context, tickTime time.Duration) error {
	shmFile, err := os.Open(shmFilePath)
	if err != nil {
		return fmt.Errorf("cannot open %s:: %w ", shmFilePath, err)
	}
	defer goutils.CloseQuietly(shmFile)
	data, err := syscall.Mmap(
		int(shmFile.Fd()),
		0,
		int(unsafe.Sizeof(Common{})),
		syscall.PROT_READ,
		syscall.MAP_SHARED,
	)
	if err != nil {
		return fmt.Errorf("mmap error:: %w", err)
	}
	defer func() { _ = syscall.Munmap(data) }()
	common := (*Common)(unsafe.Pointer(&data[0]))

	ticker := time.NewTicker(tickTime)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			utcStr := string(common.UtcDttm[:])
			for i, c := range utcStr {
				if c == 0 {
					utcStr = utcStr[:i]
					break
				}
			}
			slog.Default().Info(fmt.Sprintf("GeoCalc: Busy: %d, nsat: %d, nbs %d, utc: %s", common.Busy, common.Nsat, common.Nbs, utcStr))
		}

	}
}

func RunGeoCalc(ctx context.Context) <-chan error {
	chErr := make(chan error, 1)
	go func() {
		err := run(ctx, "stdbuf", "-oL", "-eL", "./geo_calc", "./experiment.json")
		if err != nil {
			chErr <- err
		}
		close(chErr)
	}()
	fileCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	err := WaitForFile(fileCtx, shmFilePath)
	if err != nil {
		chErr <- err
	} else {
		go func() {
			err := readFromGeoCalcBlocking(ctx, 2*time.Second)
			if err != nil {
				chErr <- errors2.Wrap(err, "readFromGeoCalc")
			}
		}()
	}
	return chErr
}
