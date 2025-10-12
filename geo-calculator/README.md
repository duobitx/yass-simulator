Trecie podejœcie

## Build
```shell
gcc -o geo_calc SGP4.c TLE.c geo_calc.c -lm -lrt

./geo_calc &
````

Program chodzi a¿ do otrzymania sygna³u kill -7.
Zak³ada obszar w RAM o nazwie geo_calc_shared_memory i co sekundê wype³nia strukturê opisan¹ w pliku common.h
Przyk³adowe dane to 149 satelitów na niskich orbitach. Dziêki temu jeden satelita widzi œrednio tylko 28 innych.
Wszystkie lataj¹ ! - znaczy nie generujj¹ b³edów w SGP4.


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

Przyk³¹dowy program reader.go wyœwietla co 2 sekundy pierwsze trzy pola ze strukury common


## Run
```go

go run reader.go
```

Busy:  0 , nsat: 149 , UTC: 2025-10-12 19:18:36

Busy:  0 , nsat: 149 , UTC: 2025-10-12 19:18:38

Busy:  0 , nsat: 149 , UTC: 2025-10-12 19:18:40

Busy:  0 , nsat: 149 , UTC: 2025-10-12 19:18:42

Busy:  0 , nsat: 149 , UTC: 2025-10-12 19:18:44


Flagê busy mo¿na wykorzystaæ jako semafor. W czasie wype³niania strukury common ma wartoœæ 1.
Ale trwa to 1 ms. Gdyby reader trafi³ na busy=1, to wystarczy, jak poczeka 2 ms.
