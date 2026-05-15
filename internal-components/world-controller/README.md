# Yass World Controller

## Responsibilities:
 * Register/unregister FsNode to Experiment-Executor via messaging.
 * Updates info about fsNode in fsnode custom resource.
 * Shares info about all nodes via `/shared/nodes.json` file.

## Traffic Control Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         NETWORK HANDLER                              │
│                    (networking.Handler)                              │
└────────────────────────────────┬────────────────────────────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  Update(networkParams)  │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  replaceIPProfile()     │
                    │  removeIPProfile()      │
                    └────────────┬────────────┘
                                 │
                  ┌──────────────┴──────────────┐
                  │                             │
         ┌────────▼────────┐         ┌─────────▼──────────┐
         │   EGRESS PATH   │         │   IP MANAGEMENT    │
         │  (Outbound)     │         │                    │
         └────────┬────────┘         └─────────┬──────────┘
                  │                            │
    ┌─────────────┼────────────────┐          │
    │             │                │          │
┌───▼───┐   ┌────▼─────┐   ┌──────▼────┐     │
│ HTB   │   │  NETEM   │   │  FLOWER   │     │
│ Class │──▶│  Qdisc   │   │  Filters  │     │
│ 1:CID │   │  CID:0   │   │           │     │
└───┬───┘   └────┬─────┘   └──────┬────┘     │
    │            │                │          │
    │     ┌──────▼──────┐         │          │
    │     │  Bandwidth  │         │          │
    │     │  Limiting   │         │          │
    │     └─────────────┘         │          │
    │                             │          │
    │     ┌───────────────┐       │          │
    │     │ Delay/Latency │       │          │
    │     │ (microseconds)│       │          │
    │     └───────────────┘       │          │
    │                             │          │
    │     ┌───────────────┐       │          │
    │     │ Packet Loss   │       │          │
    │     │  (percentage) │       │          │
    │     └───────────────┘       │          │
    │                             │          │
    └─────────────┬───────────────┘          │
                  │                          │
         ┌────────▼────────────┐    ┌────────▼────────┐
         │ Traffic Filtering:  │    │   getCID(ip)    │
         │                     │    │                 │
         │ • TCP (4000-5000,   │    │  IP → ClassID   │
         │       9000-9999)    │    │   Conversion    │
         │ • UDP (same ranges) │    │                 │
         │ • ICMP (all)        │    └─────────────────┘
         │                     │
         │ DestIP: Target Node │
         └─────────────────────┘


                  FLOW DIAGRAM
                  ════════════

NetworkParam Input                    State Management
     │                                     │
     ├─ ToIP: Target IP          ┌────────▼────────┐
     ├─ PackageLoss: 0-100%      │ state map       │
     ├─ PackageDelay: ms         │ [IP]*NetworkParam│
     └─ Bandwidth: bps           └─────────────────┘
                                         │
     ┌───────────────────────────────────▼────┐
     │  isFullyBlocking()?                    │
     │  (PackageLoss≥100 OR Bandwidth≤0)     │
     └─────┬────────────────────┬─────────────┘
           │ YES                │ NO
           │                    │
    ┌──────▼──────┐      ┌──────▼────────┐
    │  Remove IP  │      │   Apply TC    │
    │   Profile   │      │   Rules       │
    └─────────────┘      └───────────────┘
           │                    │
           │            ┌───────▼────────┐
           │            │ ClassReplace   │
           │            │ QdiscReplace   │
           │            │ FilterReplace  │
           │            └────────────────┘
           │                    │
           └────────────┬───────┘
                        │
                ┌───────▼────────┐
                │  TC Applied    │
                │  to Interface  │
                └────────────────┘
```

### Key Components

1. **HTB Class (1:CID)** - Hierarchical Token Bucket limiting bandwidth
2. **NETEM Qdisc (CID:0)** - Network Emulator adding delays and packet loss
3. **Flower Filters** - Traffic classifiers for TCP/UDP (ports 4000-5000 + 9000-9999, control-plane 8080 excluded) and ICMP
4. **CID Generation** - IP to ClassID conversion via bitwise operations with network mask

### Controlled Parameters

- **Bandwidth**: bits/s — derived from `bandwidth` field in MQTT update (proto), scales with distance via inverse-square in `experiment-executor`
- **Delay**: ms → μs
- **Loss**: 0-100%
- **Port ranges**: 4000-5000 and 9000-9999 (covers IPFS swarm 4001 TCP/UDP, tus 9090, ipfs-cluster 9094/9096). 8080 control-plane port is intentionally outside both ranges.

