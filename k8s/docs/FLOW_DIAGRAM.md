# KDS Lease Manager - Test Flow Diagram

## Initialization Flow

```
┌─────────────────────────────────────────────────────────────┐
│                        ./setup.sh                           │
└────────────┬────────────────────────────────────────────────┘
             │
             ├─> Start Minikube
             │
             ├─> Build Docker image (kds-consumer-test:latest)
             │
             ├─> Apply Kubernetes manifests:
             │   ├─> RBAC (ServiceAccount, Role, RoleBinding)
             │   ├─> ConfigMap (AWS credentials, config)
             │   ├─> LocalStack deployment
             │   ├─> Init Job (create stream + tables)
             │   └─> StatefulSet (3 consumer replicas)
             │
             └─> Environment ready!
```

## Worker Startup Flow (Each Worker)

```
┌──────────────────────────────────────────────────────────────┐
│                    Worker Pod Starts                          │
└────────────┬─────────────────────────────────────────────────┘
             │
             ├─> Load config from environment
             │   (AWS endpoint, stream name, app name)
             │
             ├─> Initialize AWS clients
             │   ├─> Kinesis client
             │   └─> DynamoDB client
             │
             ├─> Initialize KDS Lease Manager
             │   │
             │   ├─> Create metadata table (if needed)
             │   │
             │   ├─> Get current shard count
             │   │   └─> Call Kinesis ListShards API
             │   │
             │   ├─> Get current worker count  
             │   │   ├─> Try K8s API (get StatefulSet replicas)
             │   │   └─> Fallback to env var or default
             │   │
             │   ├─> Check coordinator metadata
             │   │   │
             │   │   ├─> IF coordinator exists:
             │   │   │   ├─> Check if config changed
             │   │   │   │   (shards or workers different?)
             │   │   │   ├─> IF changed:
             │   │   │   │   ├─> Calculate new max leases
             │   │   │   │   └─> Update coordinator (conditional)
             │   │   │   └─> ELSE:
             │   │   │       └─> Use existing value
             │   │   │
             │   │   └─> IF no coordinator:
             │   │       ├─> Calculate max leases
             │   │       ├─> Try create coordinator (conditional)
             │   │       │   ├─> SUCCESS: Became coordinator
             │   │       │   └─> FAIL: Another worker won, read their value
             │   │       └─> Use computed value
             │   │
             │   └─> Save worker's own metadata
             │
             ├─> Set readiness = true
             │
             └─> Run consumer loop (simulate processing)
```

## Coordinator Selection (Race Condition Handling)

```
Time: T0 - All workers start simultaneously

┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│  Worker-0    │  │  Worker-1    │  │  Worker-2    │
└──────┬───────┘  └──────┬───────┘  └──────┬───────┘
       │                 │                 │
       │ Calculate       │ Calculate       │ Calculate
       │ max_leases=10   │ max_leases=10   │ max_leases=10
       │                 │                 │
       ├─────────────────┴─────────────────┤
       │    All try to create coordinator  │
       │    with conditional write:        │
       │    attribute_not_exists(worker_id)│
       └─────────────┬─────────────────────┘
                     │
        ┌────────────┼────────────┐
        │            │            │
        ▼            ▼            ▼
   ┌────────┐  ┌─────────┐  ┌─────────┐
   │SUCCESS │  │  FAIL   │  │  FAIL   │
   │Became  │  │Condition│  │Condition│
   │Coord.  │  │Check    │  │Check    │
   └────┬───┘  └────┬────┘  └────┬────┘
        │           │             │
        │           │             │
        │      Read coordinator   │
        │      metadata ◄─────────┤
        │           │             │
        ▼           ▼             ▼
   max=10      max=10        max=10
```

## Configuration Change Detection

```
Initial State: 30 shards, 3 workers, max_leases=10

Time: T1 - Stream scaled to 60 shards
┌────────────────────────────────────────────────────┐
│  DynamoDB metadata table                           │
│  coordinator: shards=30, workers=3, max_leases=10  │
└────────────────────────────────────────────────────┘

Time: T2 - Worker-0 restarts
┌──────────────────────────────────────────────────┐
│  Worker-0 starts                                  │
│  ├─> Get shard count → 60 (NEW!)                 │
│  ├─> Get worker count → 3                        │
│  ├─> Read coordinator: shards=30, workers=3      │
│  ├─> Detect change: 30 ≠ 60 ✓                    │
│  ├─> Calculate: ceil(60/3) = 20                  │
│  └─> Update coordinator with condition:          │
│      IF shards=30 AND workers=3 THEN update       │
│         ├─> SUCCESS: Updated to max_leases=20    │
│         └─> FAIL: Another worker beat me         │
└──────────────────────────────────────────────────┘

After Update:
┌────────────────────────────────────────────────────┐
│  DynamoDB metadata table                           │
│  coordinator: shards=60, workers=3, max_leases=20  │
└────────────────────────────────────────────────────┘

Time: T3 - Worker-1 and Worker-2 restart
├─> Read coordinator: shards=60, workers=3, max=20
└─> No change detected, use max_leases=20
```

## Test Scenario Flow

### Scenario 1: test-scale-shards.sh 60

```
┌─────────────────────────────────────────────────────┐
│  1. Update Kinesis stream to 60 shards              │
│     (via LocalStack API)                            │
└────────────┬────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────┐
│  2. Query current metadata (before restart)         │
│     coordinator: shards=30, max_leases=10           │
└────────────┬────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────┐
│  3. Restart worker-0 (kubectl delete pod)           │
└────────────┬────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────┐
│  4. Worker-0 detects change and updates             │
│     coordinator: shards=60, max_leases=20           │
└────────────┬────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────┐
│  5. Query metadata (after restart)                  │
│     coordinator: shards=60, max_leases=20 ✓         │
└─────────────────────────────────────────────────────┘
```

### Scenario 2: test-scale-workers.sh 5

```
┌─────────────────────────────────────────────────────┐
│  1. Query current metadata                          │
│     coordinator: workers=3, max_leases=10           │
└────────────┬────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────┐
│  2. Scale StatefulSet to 5 replicas                 │
│     kubectl scale statefulset kds-consumer --replicas=5│
└────────────┬────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────┐
│  3. Wait for new pods: consumer-3, consumer-4       │
└────────────┬────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────┐
│  4. New workers start, detect worker count change   │
│     workers: 3 → 5, recalculate: ceil(30/5) = 6    │
└────────────┬────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────┐
│  5. First new worker updates coordinator            │
│     coordinator: workers=5, max_leases=6 ✓          │
└────────────┬────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────┐
│  6. Old workers still running with max_leases=10    │
│     (Until they restart)                            │
└─────────────────────────────────────────────────────┘
```

## Monitoring Flow

```
┌──────────────────────────────────────────────────────┐
│  ./monitor.sh (runs every 10 seconds)                │
└────────────┬─────────────────────────────────────────┘
             │
             ├─> Query DynamoDB metadata table
             │   └─> Show all workers + coordinator
             │
             ├─> Query coordinator metadata (detailed)
             │
             └─> Show pod status (kubectl get pods)
```

## Data Flow

```
┌────────────────────────────────────────────────────────────┐
│                    Data Sources                            │
│                                                            │
│  ┌──────────────┐         ┌──────────────┐               │
│  │   Kinesis    │         │  Kubernetes  │               │
│  │   Stream     │         │     API      │               │
│  │              │         │              │               │
│  │ ListShards() │         │ GetStateful  │               │
│  │   → count    │         │  Set().      │               │
│  │              │         │  Replicas    │               │
│  └──────┬───────┘         └──────┬───────┘               │
│         │                        │                        │
│         └────────────┬───────────┘                        │
│                      │                                    │
│                      ▼                                    │
│           ┌──────────────────────┐                        │
│           │  Lease Manager       │                        │
│           │  Calculate:          │                        │
│           │  ceil(shards/workers)│                        │
│           └──────────┬───────────┘                        │
│                      │                                    │
│                      ▼                                    │
│           ┌──────────────────────┐                        │
│           │  DynamoDB            │                        │
│           │  Metadata Table      │                        │
│           │  - coordinator row   │                        │
│           │  - worker rows       │                        │
│           └──────────────────────┘                        │
└────────────────────────────────────────────────────────────┘
```

---

**These diagrams illustrate the complete flow of the KDS Lease Manager test environment!**

