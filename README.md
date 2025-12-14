# yass-internal-components

This repo is a multi-module Go workspace. It contains at least the following modules:
- ./go-common (utilities, proto files)
- ./world-controller
- ./geo-calculator
- ./experiment-executor


## Prerequirements

### Taskfile support
Task tool - [taskfile.dev](https://taskfile.dev/)
```shell
sudo snap install task --classic
```

### Gcc

### Golang toolchain

### Jansson lib
```shell
sudo apt install libjansson-dev
```

### Protocol Buffers compiler
<https://protobuf.dev/installation/>

- Linux: `apt install -y protobuf-compiler`
- MacOS: `brew install protobuf`

### Make
