# geo-calculator is compiled in-line as a stage of THIS image so the binary is
# always rebuilt from the current source in lockstep with the rest of the
# image. A change to geo-calculator/V6/*.cc invalidates the g++ layer below, so
# there is no external mutable-tag builder image and no stale-binary risk.
FROM ubuntu:24.04 AS geo-calc-builder
RUN apt-get update && apt-get install -y \
    g++ \
    libjansson-dev \
    libprotobuf-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY geo-calculator/V6/libsgp4.a .
COPY geo-calculator/V6/*.h .
COPY geo-calculator/V6/*.cc .
RUN g++ -o geo_calc geo_calc.cc geocalc_message.pb.cc -L. -lsgp4 -ljansson -lprotobuf -std=c++17 -static

FROM ubuntu
LABEL author="duobit"

WORKDIR /
RUN apt-get update
# Combine multiple binaries into one container:

# Resource-to-json
COPY resource-to-json/main ./resource-to-json

# World controller
COPY world-controller/main ./world-controller

# Events webapp
COPY events-webapp/main ./events-webapp

# Experiment executor
COPY experiment-executor/main ./experiment-executor

# Experiment API service (aggregated kubernetes APIService backend)
COPY experiment-apiservice/main ./experiment-apiservice

# Metrics bridge (MQTT -> MQTT metrics aggregator)
COPY metrics-bridge/main ./metrics-bridge

# mqtt2prom (MQTT metrics -> Prometheus /metrics)
COPY mqtt2prom/main ./mqtt2prom

# Events exporter (Loki -> .ods)
COPY events-exporter/main ./events-exporter

# FUSE errorfs — mounted by world-controller to simulate DiskFailure
COPY fuse-errorfs/main ./fuse-errorfs

COPY --from=geo-calc-builder /build/geo_calc ./geo_calc
RUN apt-get install -y libjansson4

# Networking + fuse tools (for hardware-event injection)
RUN apt-get install -y iproute2 bash inetutils-ping fuse

COPY traffic.sh /
COPY world-controller-wrapper.sh /
RUN chmod +x /traffic.sh /world-controller-wrapper.sh

RUN apt-get clean && rm -rf /var/lib/apt/lists/*
EXPOSE 8080


