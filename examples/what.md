# YASS - Project Goals

What do we want to achieve?

## Questions:

1. Does the model need to take into account the position of the Earth relative to the Sun? Goal: to determine whether the satellite obtains power from solar panels.
2. Can we assume that the Earth is a perfect sphere? YES.
3. Can we simplify the model and assume that the orbits are circular? NO.
4. Does the connection between satellites provide error correction?
5. Can we assume that satellites support the IP protocol and to what extent (e.g., TCP, UDP, ICMP)?

## Assumptions:

1. The ground station is a satellite located at a distance from Earth equal to the radius of Earth and with an angular velocity equal to the angular velocity of Earth.
2. Orbital parameters defined in TLE format. Additionally, a 3-element rotation vector.
3. We take into account LOS (line of sight) between satellites. Without taking into account atmospheric disturbances, reflections, solar activity, etc.
4. We consider the Earth as an obstacle to LOS.

## Objectives

1. For each pair of satellites, determine whether the satellites can see each other and at what distance.
2. Determine whether the satellite is in low power mode f(t).
3. Information from points 1 and 2 is needed to determine the communication parameters at time t.

## Data model

1. Positions of each satellite in f(t)
2. Rotation of each satellite in f(t)
3. Hardware resources of each satellite

- CPU, RAM, Storage
- Amount of energy (battery, solar panels)
- Communication system parameters (range, bandwidth, etc.)

# Technical

1. We always expose the same ports 30001-30010
2. N simulations on different layouts and configurations are N separate experiments.

# Experiment model elements:

- Experiment Definition:
  - Node behaviors events
  - Hardware events
  - Layout
    - Orbit
    - Layout-Node-Template
      - Hardware profile
