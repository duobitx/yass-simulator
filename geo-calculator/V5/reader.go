package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const MAXSAT = 200

type SatPosDescr struct {
	Name   [32]byte
	X      float64
	Y      float64
	Z      float64
	Lat    float64
	Lng    float64
	Alt    float64
	NRef   int32
	SatRef [MAXSAT]SatRefDscr
}

type SatRefDscr struct {
	Sid  int32
	Dist float32
}

type Common struct {
	Busy    int32
	Nsat    int32
	Nbs     int32
	UtcDttm [32]byte
	Sats    [MAXSAT]SatPosDescr
}

func main() {
	path := "/dev/shm/geo_calc_shared_memory" // POSIX shm_open tworzy plik o tej nazwie
	file, err := os.Open(path)
	if err != nil {
		fmt.Println("Błąd otwierania shm:", err)
		return
	}
	defer file.Close()

	// Mapujemy do pamięci
	data, err := syscall.Mmap(
		int(file.Fd()),
		0,
		int(unsafe.Sizeof(Common{})),
		syscall.PROT_READ,
		syscall.MAP_SHARED,
	)
	if err != nil {
		fmt.Println("Błąd mmap:", err)
		return
	}
	defer syscall.Munmap(data)

	// Interpretujemy dane jako strukturę
	common := (*Common)(unsafe.Pointer(&data[0]))

	for i := 0; i < 5; i++ {
		utcStr := string(common.UtcDttm[:])
		for i, c := range utcStr {
			if c == 0 {
				utcStr = utcStr[:i]
				break
			}
		}
		fmt.Println("Busy: ", common.Busy, ", nsat:", common.Nsat, ", nbs:", common.Nbs, " UTC:", utcStr)
		for i := 0; i < MAXSAT; i++ {
			sat := common.Sats[i]
			satName := convBytesToString(sat.Name[:])
			fmt.Printf("Sat %d %s -- %+v\n", i, satName, sat)
		}
		time.Sleep(2 * time.Second)
	}
}

func convBytesToString(buff []byte) string {
	j := len(buff) - 1
	for i := 0; i < len(buff); i++ {
		if buff[i] == 0 {
			j = i
			break
		}
	}
	buff = buff[:j]
	return string(buff)
}
