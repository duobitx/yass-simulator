FROM ubuntu
LABEL author="duobit"

WORKDIR /
RUN apt-get update
# Combine multiple binaries into one container:

# Resource-to-json
COPY resource-to-json/main ./resource-to-json

# World controller
COPY world-controller/main ./world-controller

# Experiment executor
COPY experiment-executor/main ./experiment-executor
COPY geo-calculator/geo_calc ./geo_calc
RUN apt-get install -y libjansson4

# Networking tools
RUN apt-get install -y iproute2 bash

COPY traffic.sh /
RUN chmod +x /traffic.sh

RUN apt-get clean && rm -rf /var/lib/apt/lists/*
EXPOSE 8080


