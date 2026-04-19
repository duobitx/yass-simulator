# Example of protobuf usage

## cpp/example.cpp
`cpp/example.cpp` generates a binary file `common.bin` that is prefixed with a length of the structure.

### Compile
```shell
g++ -o example cpp/example.cpp ../../go-common/proto/cpp/geocalc_message.pb.cc -I../../go-common/proto -lprotobuf -std=c++17
```
### Run
```shell
./example
```



## Reader.go
Reader.go reads the file.

### Compile and run
```shell
go run reader.go
```