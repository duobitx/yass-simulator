FROM busybox
LABEL author="duobit"

WORKDIR /

# Combine multiple binaries into one container:

# Resource-to-json
COPY resource-to-json/main ./resource-to-json

# World controller
COPY world-controller/main ./world-controller

# Experiment executor
COPY experiment-executor/main ./experiment-executor
EXPOSE 8080


