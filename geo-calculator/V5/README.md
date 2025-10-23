Wersja w weœciem z uzgodnionego JSON-a

## Build
```shell

porzebna biblioteka jansson  (sudo apt install libjansson-dev)

g++ -o geo_calc geo_calc.cc   -L. -lsgp4 -ljansson

./geo_calc <plik json> &


````

przyk³¹dowy plik: examp149.json


Program chodzi a¿ do otrzymania sygna³u kill -7.
Zak³ada obszar w RAM o nazwie geo_calc_shared_memory i co sekundê wype³nia strukturê opisan¹ w pliku common.h
uwaga - zmiana w strukturze - dosz³y nazwy

## Struct
```c
#ifndef COMMON_H
#define COMMON_H

#define MAXSAT 200

struct sat_ref_dscr { int sid; float dist; };

struct sat_pos_dscr {
	 char name[32];
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
Flagê busy mo¿na wykorzystaæ jako semafor. W czasie wype³niania strukury common ma wartoœæ 1.
Ale trwa to 1 ms. Gdyby reader trafi³ na busy=1, to wystarczy, jak poczeka 2 ms.


```
Tabela sat zawiera:
  satelity - indeksy 0..nsat-1
  stacje bazowe - indeksy nsat...nsat+nbs-1
```

Algorytm wyliczania wspó³rzêdnych jest inny dla satelitów i inny dla staci bazowych.
Dla satelitów wylicza co sekundê po³o¿enie w dwóch uk³adach wspó³rzêdnych:  (x,y,z) i (lat,lon, alt).
Dla stacji bazowych (zadane  lat,lon, alt ) wylicza co sekundê x, y, z (choæ z siê nie zmienia).

Algorytm badania widzialnoœci jest obecnie ten sam dla par satelitów jak i satelita-stacja bazowa.
Jestem przygotowany do jego rozbudowy (np. sto¿ek ).

W szczególnoœci dla stacji bazowej dostajemy w sat_ref listê nref widocznych satelitów.

Przyk³adowe dane to 149 satelitów na niskich orbitach. Dziêki temu jeden satelita widzi œrednio tylko 28 innych.
Ze stacji bazowych  widaæ równoczeœnie 4 do 12 satelitów.


Przyk³¹dowy program reader.go wyœwietla co 2 sekundy pierwsze trzy pola ze strukury common

## Run
```go

go run reader.go
```

