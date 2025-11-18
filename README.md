# Mohan's KCL Rebalancing Experiment

This experiment demonstrates advanced Kinesis Data Streams (KDS) rebalancing scenarios with KCL (Kinesis Client Library) consumers.

## Requirements Overview

1. **Producer**: Send data to Kinesis LocalStack with 20 shards
2. **3 KCL Consumers**: Initially consume data from 20 shards
3. **Shard Split**: Increase shards by 50% (20 → 30 shards)
4. **Scale Consumers**: Scale from 3 to 5 Kubernetes pods
5. **Lease Stealing**: Use `EnableLeaseStealing = true` with `MaxLeasesForWorker = min(10, NumShards/NumPODs)` algorithm


## Quick Start

### 1. Start LocalStack with 20 Shards

```bash
# Start LocalStack
docker-compose -f docker-compose-20.yml up -d

# Wait for initialization (creates stream with 20 shards)
sleep 10

# Verify stream
./scripts/verify-stream.sh
```

### 2. Start Producer

```bash
# Terminal 1: Start producer for 20 shards
make producer
```

### 3. Start 3 KCL Consumers

```bash
# Terminal 2: Consumer Pod 1
make consumer-pod1

# Terminal 3: Consumer Pod 2
make consumer-pod2

# Terminal 4: Consumer Pod 3
make consumer-pod3
```

### 4. Monitor Lease Distribution

```bash
# Terminal 5: Watch lease distribution
./scripts/monitor-leases.sh
```

### 5. Perform Shard Split (50% Increase: 20 → 30)

```bash
# This will split shards to increase from 20 to 30
./scripts/split-shards.sh
```

### 6. Scale to 5 Consumers

```bash
# Terminal 6: Consumer Pod 4
make consumer-pod4

# Terminal 7: Consumer Pod 5
make consumer-pod5
```

### 7. Observe Rebalancing

Watch the lease distribution adjust automatically with:
- Lease stealing enabled
- MaxLeasesForWorker = min(10, 30/5) = 6 leases per pod


## Configuration Details

### Lease Stealing Algorithm

```
MaxLeasesForWorker = min(10, NumShards / NumPODs)

With 20 shards and 3 pods: max = min(10, 20/3) = 6 leases/pod
With 30 shards and 5 pods: max = min(10, 30/5) = 6 leases/pod
```

### Key Settings

- `EnableLeaseStealing`: true
- `LeaseStealing.StealingStrategy`: EVEN_DISTRIBUTION
- `ShardSyncIntervalMillis`: 5000 (5 seconds)


## Monitoring

### View Shard Distribution

```bash
./scripts/view-distribution.sh
```

### Export DynamoDB Lease Data

```bash
./scripts/export-lease-data.sh > leases.json
```

### Analyze Rebalancing Performance

```bash
./scripts/analyze-rebalancing.sh
```

## Architecture

```
┌─────────────┐
│  Producer   │  → Produces data to 20/30 shards
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────┐
│   Kinesis Stream (LocalStack)   │
│   Initial: 20 shards             │
│   After Split: 30 shards         │
└──────────┬──────────────────────┘
           │
           ▼
┌──────────────────────────────────┐
│   DynamoDB Lease Table           │
│   - Tracks shard ownership       │
│   - Enables lease stealing       │
│   - Coordinates rebalancing      │
└──────────┬───────────────────────┘
           │
           ▼
┌─────────────────────────────────────┐
│   KCL Consumer Pods                 │
│   Initial: 3 pods (6-7 leases each) │
│   Scaled: 5 pods (6 leases each)    │
│   - EnableLeaseStealing: true       │
│   - MaxLeases: min(10, N/P)         │
└─────────────────────────────────────┘
```

## Experiment Steps

### Phase 1: Initial Setup (20 shards, 3 consumers)

1. LocalStack creates stream with 20 shards
2. 3 consumers start and acquire leases
3. Each consumer gets ~6-7 shards
4. Producer sends data continuously

### Phase 2: Shard Split (20 → 30 shards)

1. Script splits shards by 50%
2. Creates 10 new child shards
3. Parent shards are marked as closed (for new writes)
4. Child shards immediately available for new write
5. Consumers detect new shards within 5 seconds (configurable)

### Phase 3: Consumer Scale-Up (3 → 5 pods)

1. Start 2 additional consumer pods
2. Lease stealing triggers rebalancing
3. Target: 6 leases per pod (30/5)
4. Pods with >6 leases release extras
5. New pods steal available leases
6. Converges to even distribution

## Expected Behavior

### Lease Distribution Timeline

```
Time 0s:   3 pods, 20 shards
           Pod1: 7, Pod2: 7, Pod3: 6

Time 60s:  Shard split initiated (20 → 30)
           Split creates 10 new children
           
Time 65s:  Child shards detected
           Pod1: 7 + new, Pod2: 7 + new, Pod3: 6 + new
           
Time 120s: Pods 4 & 5 started
           Pod1: 7, Pod2: 7, Pod3: 6, Pod4: 0, Pod5: 0
           + 10 new child shards available

Time 130s: Lease stealing begins
           Pods with >6 leases release extras
           Pods 4 & 5 start acquiring leases

Time 180s: Converged (ideal state)
           Pod1: 6, Pod2: 6, Pod3: 6, Pod4: 6, Pod5: 6
```


## Troubleshooting

### Shards Not Splitting

```bash
# Check stream status
./scripts/verify-stream.sh

# Manual split
./scripts/split-shards.sh --force
```

### Leases Not Rebalancing

```bash
# Check DynamoDB lease table
./scripts/export-lease-data.sh

# Verify consumer configuration
cat config/config-pod*.yaml | grep -A 5 lease
```




