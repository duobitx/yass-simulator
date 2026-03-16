Third approach

## Build
```shell
gcc -o geo_calc SGP4.c TLE.c geo_calc.c -lm -lrt

./geo_calc &
````

The program runs until it receives a kill -7 signal.
It creates a RAM area named geo_calc_shared_memory and fills the structure described in common.h every second.
The example data consists of 149 satellites in low orbits. Thanks to this, one satellite sees an average of only 28 others.
All are flying! - meaning they don't generate errors in SGP4.


## Struct
```c
#ifndef COMMON_H
#define COMMON_H

#define MAXSAT 200

struct sat_ref_dscr { int sid; float dist; };

struct sat_pos_dscr {
   double x, y, z, h;
   int nref;
   struct sat_ref_dscr sat_ref[MAXSAT];
 };


struct common  {
    int busy, nsat;
    char utc_dttm[32];
    struct sat_pos_dscr sat[MAXSAT];
};

#endif

```

The example reader.go program displays the first three fields from the common structure every 2 seconds.


## Run
```go

go run reader.go
```

Busy:  0 , nsat: 149 , UTC: 2025-10-12 19:18:36

Busy:  0 , nsat: 149 , UTC: 2025-10-12 19:18:38

Busy:  0 , nsat: 149 , UTC: 2025-10-12 19:18:40

Busy:  0 , nsat: 149 , UTC: 2025-10-12 19:18:42

Busy:  0 , nsat: 149 , UTC: 2025-10-12 19:18:44


The busy flag can be used as a semaphore. While filling the common structure, it has a value of 1.
But this takes 1 ms. If the reader encounters busy=1, it's enough to wait 2 ms.
