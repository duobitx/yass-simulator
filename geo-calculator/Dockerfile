FROM ubuntu:24.04 AS geo-calc-builder

RUN apt-get update && apt-get install -y \
    g++ \
    libjansson-dev \
    libprotobuf-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

COPY V6/libsgp4.a . 
COPY V6/*.h .
COPY V6/*.cc .

#RUN g++ -o ./geo_calc geo_calc.cc -L. -lsgp4 -ljansson
RUN g++ -o geo_calc geo_calc.cc geocalc_message.pb.cc  -L. -lsgp4 -ljansson -lprotobuf -std=c++17 -static

