Wersja w weœciem z uzgodnionego JSON-a

## Build
```shell

porzebna biblioteka jansson  (sudo apt install libjansson-dev)

g++ -o geo_calc geo_calc.cc geocalc_message.pb.cc  -L. -lsgp4 -ljansson -lprotobuf -std=c++17

wywo³anie:

./geo_calc <plik json>  &
lub
./geo_calc <plik json> <rrrr-mm-ddThh:mm:ss>


W przypadku u¿ycia drugiego parametru program ignoruje zawarte w json dane o symulowanym czasie, wylicza po³o¿enie satelitów dla wskazanegi timestamp i koñczy sie.Ale wyœwietla sposób wype³nienia serializowanych danych.

Przyk³adowo:

./geo_calc examp_marekg.json 2025-12-14T04:00:00


Parsing file: examp_marekg.json
  experiment name:  netowking-demo-ping-experiment
  n_sat:     3
  n_bs:      1
Filling SGP4 structures
  n_sat= 3 (number of verified satellites)

shared memory:
nsat:   3
nbs:    1
utc_dttm: 2025-12-14 04:00:00,    sun:  lat=  -23.22, lon=  118.65,   vx= -40489.46, vy=-278642.64, vz=-120765.93
id:  1, name=oneweb-0008              x=   -619.91, y=  -5729.31, z=  -4928.60, lat=  -40.70, lon=  120.74, alt=   1213.82, InTheSun=1
id:  2, name=yaogan-25c               x=  -2339.92, y=  -7280.44, z=  -1141.81, lat=   -8.54, lon=  109.10, alt=   1354.33, InTheSun=1
id:  3, name=kuiper-00060             x=   3874.85, y=  -1923.94, z=   5501.88, lat=   51.99, lon= -169.49, alt=    634.15, InTheSun=1
id:  4, name=new-norcia               x=   -962.98, y=  -5424.79, z=  -3202.46, lat=  -30.33, lon=  116.85, alt=      0.00, InTheSun=1
item_id_a:   1, item_id_b:   2, distance:  4438.94, los: 1
item_id_a:   2, item_id_b:   1, distance:  4438.94, los: 1
item_id_a:   1, item_id_b:   3, distance: 11978.25, los: 0
item_id_a:   3, item_id_b:   1, distance: 11978.25, los: 0
item_id_a:   1, item_id_b:   4, distance:  1786.05, los: 1
item_id_a:   4, item_id_b:   1, distance:  1786.05, los: 1
item_id_a:   2, item_id_b:   3, distance: 10557.17, los: 0
item_id_a:   3, item_id_b:   2, distance: 10557.17, los: 0
item_id_a:   2, item_id_b:   4, distance:  3096.08, los: 1
item_id_a:   4, item_id_b:   2, distance:  3096.08, los: 1
item_id_a:   3, item_id_b:   4, distance: 10555.86, los: 0
item_id_a:   4, item_id_b:   3, distance: 10555.86, los: 0

Number of sunny satellites: 3
avg nref:  1.5

Message contains: 4 items, 6 distances
Required buffer size : 366 + 5


````

przyk³¹dowy plik: examp149.json


Program chodzi a¿ do otrzymania sygna³u kill -7.
Zak³ada obszar w RAM o nazwie geo_calc_shared_memory i co sekundê serializuje wyniki zgodnie z definicj¹ w pliku geocalc_message.proto.

nsat - liczba wczytanych i zweryfikowanych satelitów.
nbs - liczba staci bazowych
Flagê busy mo¿na wykorzystaæ jako semafor. W czasie wype³niania strukury common ma wartoœæ 0.
Ale trwa to 1 ms. Gdyby reader trafi³ na busy=0, to wystarczy, jak poczeka 2 ms.


```
Tabela items zawiera:
  satelity - item_id w zakresie 1..nsat
  stacje bazowe - item_id w zakresie nsat+1..nsat+nbs
```

Algorytm wyliczania wspó³rzêdnych jest inny dla satelitów i inny dla staci bazowych.
Dla satelitów wylicza co sekundê po³o¿enie w dwóch uk³adach wspó³rzêdnych:  (x,y,z) i (lat,lon, alt).
Dla stacji bazowych (zadane  lat,lon, alt ) wylicza co sekundê x, y, z (choæ z siê nie zmienia).

Algorytm badania widzialnoœci jest obecnie ten sam dla par satelitów jak i satelita-stacja bazowa.


Przyk³adowe dane to 149 satelitów na niskich orbitach. Dziêki temu jeden satelita widzi œrednio tylko 28 innych.
Ze stacji bazowych  widaæ równoczeœnie 4 do 12 satelitów.




## Line-of-sight model (supersedes the note above)

Visibility is no longer identical for sat-sat and sat-GS pairs:

- **sat <-> sat**: a link is clear when the straight segment between the two
  satellites does not pass below the Earth sphere (`radiusearthkm`).
- **sat <-> ground station**: the satellite must additionally be at least
  `MIN_GS_ELEVATION_DEG` (10 deg) above the ground station's local horizon.
  A real ground antenna cannot work at the limb (atmospheric attenuation,
  terrain masking, mechanical limits), so links below 10 deg of elevation are
  treated as blocked. This subsumes the Earth-occlusion test (elevation >= 0
  already means the line clears the Earth).

Note on the Earth model: ground-station positions come from libsgp4's WGS72
**ellipsoid** (with flattening), while the sat-sat occlusion test uses a single
**spherical** radius equal to the equatorial radius (6378.137 km). Near the
poles the real surface is ~21 km closer to the centre, so the two models are
inconsistent by ~0.3% in the worst case. This is accepted as negligible for the
coarse occlusion test and is not corrected.
