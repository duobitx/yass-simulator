#include <fstream>
#include <iostream>
#include "../proto/cpp/geocalc_message.pb.h"

bool WriteGeoCommonToFile(const yass::fs::GeoCommon& message, const std::string& filename) {
    std::ofstream output(filename, std::ios::binary);
    if (!output) {
        return false;
    }

    // Serialize to string
    std::string serialized;
    if (!message.SerializeToString(&serialized)) {
        return false;
    }

    // Write size as 4-byte prefix (uint32_t)
    uint32_t size = static_cast<uint32_t>(serialized.size());
    output.write(reinterpret_cast<const char*>(&size), sizeof(size));

    // Write the serialized message
    output.write(serialized.data(), serialized.size());

    return output.good();
}

int main() {
    // Create GeoCommon message with random data
    yass::fs::GeoCommon message;

    // Set timestamp
    auto* timestamp = message.mutable_time();
    timestamp->set_seconds(1710604800); // 2024-03-16 12:00:00 UTC
    timestamp->set_nanos(500000000);

    // Add items (satellites/objects)
    auto* item1 = message.add_items();
    item1->set_id(1);
    item1->set_name("ISS");
    item1->set_x(6500000.0);
    item1->set_y(1200000.0);
    item1->set_z(-500000.0);
    item1->set_lat(45.5);
    item1->set_lon(-122.6);
    item1->set_alt(408000.0);
    item1->set_in_the_sun(true);

    auto* item2 = message.add_items();
    item2->set_id(2);
    item2->set_name("Hubble");
    item2->set_x(5800000.0);
    item2->set_y(-2100000.0);
    item2->set_z(3200000.0);
    item2->set_lat(28.5);
    item2->set_lon(-80.6);
    item2->set_alt(547000.0);
    item2->set_in_the_sun(false);

    auto* item3 = message.add_items();
    item3->set_id(3);
    item3->set_name("Starlink-1234");
    item3->set_x(6200000.0);
    item3->set_y(500000.0);
    item3->set_z(-1800000.0);
    item3->set_lat(53.0);
    item3->set_lon(10.0);
    item3->set_alt(550000.0);
    item3->set_in_the_sun(true);

    // Add distances between items
    auto* dist1 = message.add_distances();
    dist1->set_item_id_a(1);
    dist1->set_item_id_b(2);
    dist1->set_distance(1852340.5);
    dist1->set_los(true);

    auto* dist2 = message.add_distances();
    dist2->set_item_id_a(1);
    dist2->set_item_id_b(3);
    dist2->set_distance(987654.3);
    dist2->set_los(false);

    auto* dist3 = message.add_distances();
    dist3->set_item_id_a(2);
    dist3->set_item_id_b(3);
    dist3->set_distance(2345678.9);
    dist3->set_los(true);

    // Write to file
    if (WriteGeoCommonToFile(message, "common.bin")) {
        std::cout << "Successfully wrote GeoCommon message to common.bin" << std::endl;
        std::cout << "Message contains:" << std::endl;
        std::cout << "  - " << message.items_size() << " items" << std::endl;
        std::cout << "  - " << message.distances_size() << " distances" << std::endl;
        return 0;
    } else {
        std::cerr << "Failed to write message to file" << std::endl;
        return 1;
    }
}
