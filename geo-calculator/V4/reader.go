package main

import (
    "fmt"
    "os"
    "syscall"
    "unsafe"
    "time"
)

type Common struct {
    Busy    int32
    Nsat    int32
    Nbs     int32
    UtcDttm [32]byte
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
      time.Sleep(2 * time.Second)
    }
}
 
