Kolejne podejœcie - wspó³rzêdne geodezyjne i  stacja bazowa

## Build
```shell
g++ -o geo_calc geo_calc.cc   -L. -lsgp4

./geo_calc [<start ekperymentu> [<wsp. akceleracji>] ]&

<start eksperymentu> w formacie yyyy-mm-ddThh:mm  - bez sekund
<wsp. akceleracji> - liczba rzeczywista >= 1.0 (domyœlnie 1.0)
````

Program chodzi a¿ do otrzymania sygna³u kill -7.
Zak³ada obszar w RAM o nazwie geo_calc_shared_memory i co sekundê wype³nia strukturê opisan¹ w pliku common.h


## Struct
```c
#ifndef COMMON_H
#define COMMON_H

#define MAXSAT 200

struct sat_ref_dscr { int sid; float dist; };

struct sat_pos_dscr {
   double x, y, z, lat, lon, alt;
   int nref;
   struct sat_ref_dscr sat_ref[MAXSAT];  
 };
 

struct common  {
    int busy, nsat, nbs;
    char utc_dttm[32];
    struct sat_pos_dscr sat[MAXSAT];
};

#endif

```

nsat - liczba wczytanych i zweryfikowanych satelitów.
nbs - liczba staci bazowych

program uzupe³nia (tymczasowo) listê satelitów o jedn¹ stacjê bazow¹. Znajduje siê w Warszawie na szczycie Kopy Cwila.

czyli tabela sat zawiera:
  satelity - indeksy 0..nsat-1
  stacje bazowe - indeksy nsat...nsat+nbs-1

Algorytm wyliczania wspó³rzêdnych jest inny dla satelitów i inny dla staci bazowych.
Dla satelitów wylicza co sekundê po³o¿enie w dwóch uk³adach wspó³rzêdnych:  (x,y,z) i (lat,lon, alt).
Dla stacji bazowych (zadane  lat,lon, alt ) wylicza co sekundê x, y, z (choæ z siê nie zmienia).

Algorytm badania widzialnoœci jest obecnie ten sam dla par satelitów jak i satelita-stacja bazowa.
Jestem przygotowany do jego rozbudowy (np. sto¿ek ).

W szczególnoœci dla stacji bazowej dostajemy w sat_ref listê nref widocznych satelitów.

Przyk³adowe dane to 149 satelitów na niskich orbitach. Dziêki temu jeden satelita widzi œrednio tylko 28 innych.
Z Warszawy widaæ równoczeœnie 4 do 9 satelitów.


Przyk³¹dowy program reader.go wyœwietla co 2 sekundy pierwsze trzy pola ze strukury common

## Run
```go

go run reader.go
```

poni¿sz wynik pochodzi z przebiegu: ./geo_calc 2025-09-01T12:00 10 &


Busy:  0 , nsat: 149 , nbs: 1  UTC: 2025-09-01 12:16:20

Busy:  0 , nsat: 149 , nbs: 1  UTC: 2025-09-01 12:16:40

Busy:  0 , nsat: 149 , nbs: 1  UTC: 2025-09-01 12:17:00

Busy:  0 , nsat: 149 , nbs: 1  UTC: 2025-09-01 12:17:20

Busy:  0 , nsat: 149 , nbs: 1  UTC: 2025-09-01 12:17:40

widaæ, ¿e co 2 sekundy czas symulowanyy skacze o 20 sekund.

Flagê busy mo¿na wykorzystaæ jako semafor. W czasie wype³niania strukury common ma wartoœæ 1.
Ale trwa to 1 ms. Gdyby reader trafi³ na busy=1, to wystarczy, jak poczeka 2 ms.
