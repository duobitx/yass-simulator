FROM ubuntu:22.04 AS geo-calc-builder

RUN apt-get update && apt-get install -y \
    g++ \
    libjansson-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

COPY V5/libsgp4.a V5/
COPY V5/*.h V5/
COPY V5/geo_calc.cc V5/

RUN cd V5 && g++ -o geo_calc geo_calc.cc -L. -lsgp4 -ljansson


