package main

import (
	"encoding/binary"
	"fmt"
	"os"

	pb "github.com/duobitx/yass-simulator/internal-components/go-common/proto/go"
	"google.golang.org/protobuf/proto"
)

func readGeoCommonFromFile(filename string) (*pb.GeoCommon, error) {
	// Open file
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	// Read size prefix (4-byte uint32)
	var size uint32
	if err := binary.Read(file, binary.LittleEndian, &size); err != nil {
		return nil, err
	}

	// Read the serialized data
	data := make([]byte, size)
	if _, err := file.Read(data); err != nil {
		return nil, err
	}

	// Deserialize the message
	message := &pb.GeoCommon{}
	if err := proto.Unmarshal(data, message); err != nil {
		return nil, err
	}

	return message, nil
}

func main() {
	message, err := readGeoCommonFromFile("common.bin")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully read GeoCommon message from common.bin\n")
	fmt.Printf("Timestamp: %v\n", message.Time.AsTime())
	fmt.Printf("Items (%d):\n", len(message.Items))
	for _, item := range message.Items {
		fmt.Printf("  ID: %d, Name: %s, Lat: %.2f, Lon: %.2f, Alt: %.0f, InSun: %v\n",
			item.Id, item.Name, item.Lat, item.Lon, item.Alt, item.InTheSun)
	}
	fmt.Printf("Distances (%d):\n", len(message.Distances))
	for _, dist := range message.Distances {
		fmt.Printf("  %d <-> %d: %.2f km, LOS: %v\n",
			dist.ItemIdA, dist.ItemIdB, dist.Distance/1000, dist.Los)
	}
}
