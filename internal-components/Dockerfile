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

# Metrics bridge (MQTT -> Prometheus)
COPY metrics-bridge/main ./metrics-bridge

# Events exporter (Loki -> .ods)
COPY events-exporter/main ./events-exporter

# FUSE errorfs — mounted by world-controller to simulate DiskFailure
COPY fuse-errorfs/main ./fuse-errorfs

COPY --from=docker.io/library/geo-calc-builder /build/geo_calc ./geo_calc
RUN apt-get install -y libjansson4

# Networking + fuse tools (for hardware-event injection)
RUN apt-get install -y iproute2 bash inetutils-ping fuse

COPY traffic.sh /
COPY world-controller-wrapper.sh /
RUN chmod +x /traffic.sh /world-controller-wrapper.sh

RUN apt-get clean && rm -rf /var/lib/apt/lists/*
EXPOSE 8080


