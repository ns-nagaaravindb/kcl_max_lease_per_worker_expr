# KDS Lease Manager - Execution Details

This document provides a step-by-step walkthrough of how the KDS Lease Manager works with DynamoDB metadata tracking, showing the actual table state at each step.

## Scenario Overview

**Initial State**: 30 shards, 3 workers  
**Scaled State**: 60 shards, 3 workers (shard count doubled)

**Formula**: `max_leases_per_worker = min(80, ceil(shard_count / worker_count))`

---

## Phase 1: Initial Deployment (30 Shards, 3 Workers)

### Step 1: Worker-1 Starts First

**Actions by Worker-1:**
1. Calls `InitializeMaxLeasesPerWorker(ctx)`
2. Creates DynamoDB table `<stream_name>_meta` if it doesn't exist
3. Calls `GetShardCount()` → returns **30**
4. Calls `GetWorkerCount()` → returns **3** (from K8s API or env var)
5. Calls `GetCoordinatorMetadata()` → returns **nil** (no coordinator exists yet)
6. Calculates: `max_leases = ceil(30/3) = 10`
7. Calls `TryCreateCoordinatorMetadata()` with conditional write (attribute_not_exists)
   - **SUCCESS** - Worker-1 becomes the coordinator
8. Saves worker-1's own metadata

**DynamoDB Table State After Step 1:**

| worker_id | max_leases_per_worker | stream_name | app_name | shard_count | worker_count | last_update_time |
|-----------|----------------------|-------------|----------|-------------|--------------|------------------|
| `my-app_coordinator` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:00Z |
| `worker-1` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:00Z |

**Result**: Worker-1 uses **max_leases_per_worker = 10**

---

### Step 2: Worker-2 Starts (2 seconds later)

**Actions by Worker-2:**
1. Calls `InitializeMaxLeasesPerWorker(ctx)`
2. Table already exists (no-op)
3. Calls `GetShardCount()` → returns **30**
4. Calls `GetWorkerCount()` → returns **3**
5. Calls `GetCoordinatorMetadata()` → returns existing coordinator metadata:
   ```
   {
     WorkerID: "my-app_coordinator",
     MaxLeasesPerWorker: 10,
     ShardCount: 30,
     WorkerCount: 3
   }
   ```
6. Checks if config changed: `30 == 30 && 3 == 3` → **No change**
7. Uses existing coordinator value: **max_leases = 10**
8. Saves worker-2's own metadata

**DynamoDB Table State After Step 2:**

| worker_id | max_leases_per_worker | stream_name | app_name | shard_count | worker_count | last_update_time |
|-----------|----------------------|-------------|----------|-------------|--------------|------------------|
| `my-app_coordinator` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:00Z |
| `worker-1` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:00Z |
| `worker-2` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:02Z |

**Result**: Worker-2 uses **max_leases_per_worker = 10**

---

### Step 3: Worker-3 Starts (4 seconds later)

**Actions by Worker-3:**
1. Calls `InitializeMaxLeasesPerWorker(ctx)`
2. Table already exists (no-op)
3. Calls `GetShardCount()` → returns **30**
4. Calls `GetWorkerCount()` → returns **3**
5. Calls `GetCoordinatorMetadata()` → returns existing coordinator metadata (max_leases=10)
6. Checks if config changed: `30 == 30 && 3 == 3` → **No change**
7. Uses existing coordinator value: **max_leases = 10**
8. Saves worker-3's own metadata

**DynamoDB Table State After Step 3:**

| worker_id | max_leases_per_worker | stream_name | app_name | shard_count | worker_count | last_update_time |
|-----------|----------------------|-------------|----------|-------------|--------------|------------------|
| `my-app_coordinator` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:00Z |
| `worker-1` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:00Z |
| `worker-2` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:02Z |
| `worker-3` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:04Z |

**Result**: Worker-3 uses **max_leases_per_worker = 10**

**System State**: All 3 workers are running with max_leases=10. Each worker can handle up to 10 shards. Total capacity = 30 leases across 3 workers, which perfectly matches 30 shards.

---

## Phase 2: Shard Count Increases to 60

### Step 4: AWS Admin Increases Shards from 30 to 60

Time: 2025-11-19T12:00:00Z (2 hours later)

**Actions**: 
- AWS Kinesis stream is scaled from 30 shards to 60 shards
- All 3 workers are still running with the old config (max_leases=10)

**DynamoDB Table State** (unchanged):

| worker_id | max_leases_per_worker | stream_name | app_name | shard_count | worker_count | last_update_time |
|-----------|----------------------|-------------|----------|-------------|--------------|------------------|
| `my-app_coordinator` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:00Z |
| `worker-1` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:00Z |
| `worker-2` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:02Z |
| `worker-3` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:04Z |

**Problem**: Workers are configured for max 10 leases each (30 total), but there are now 60 shards. The system is under-provisioned!

---

### Step 5: Worker-1 Restarts (Detects Change)

Time: 2025-11-19T12:05:00Z

**Trigger**: Worker-1 crashes or is restarted (pod restart, deployment rollout, etc.)

**Actions by Worker-1:**
1. Calls `InitializeMaxLeasesPerWorker(ctx)`
2. Table already exists (no-op)
3. Calls `GetShardCount()` → returns **60** (NEW!)
4. Calls `GetWorkerCount()` → returns **3**
5. Calls `GetCoordinatorMetadata()` → returns:
   ```
   {
     WorkerID: "my-app_coordinator",
     MaxLeasesPerWorker: 10,
     ShardCount: 30,
     WorkerCount: 3
   }
   ```
6. Checks if config changed: `30 != 60 || 3 == 3` → **CHANGE DETECTED!**
7. Calculates NEW max_leases: `ceil(60/3) = 20`
8. Calls `UpdateCoordinatorMetadata()` with conditional update:
   ```
   Condition: shard_count = 30 AND worker_count = 3
   New Values: max_leases=20, shard_count=60, worker_count=3
   ```
   - **SUCCESS** - Coordinator metadata is updated
9. Saves worker-1's own metadata with new values

**DynamoDB Table State After Step 5:**

| worker_id | max_leases_per_worker | stream_name | app_name | shard_count | worker_count | last_update_time |
|-----------|----------------------|-------------|----------|-------------|--------------|------------------|
| `my-app_coordinator` | **20** | kds-stream-prod | my-app | **60** | 3 | **2025-11-19T12:05:00Z** |
| `worker-1` | **20** | kds-stream-prod | my-app | **60** | 3 | **2025-11-19T12:05:00Z** |
| `worker-2` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:02Z |
| `worker-3` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:04Z |

**Result**: Worker-1 now uses **max_leases_per_worker = 20**

**Note**: Worker-2 and Worker-3 are still running with old config (max_leases=10). KCL on these workers will eventually pick up new shards, but they're limited to 10 leases each until they restart.

---

### Step 6: Worker-2 Restarts (5 seconds later)

Time: 2025-11-19T12:05:05Z

**Actions by Worker-2:**
1. Calls `InitializeMaxLeasesPerWorker(ctx)`
2. Table already exists (no-op)
3. Calls `GetShardCount()` → returns **60**
4. Calls `GetWorkerCount()` → returns **3**
5. Calls `GetCoordinatorMetadata()` → returns:
   ```
   {
     WorkerID: "my-app_coordinator",
     MaxLeasesPerWorker: 20,
     ShardCount: 60,
     WorkerCount: 3
   }
   ```
6. Checks if config changed: `60 == 60 && 3 == 3` → **No change** (already updated by Worker-1)
7. Uses existing coordinator value: **max_leases = 20**
8. Saves worker-2's own metadata

**DynamoDB Table State After Step 6:**

| worker_id | max_leases_per_worker | stream_name | app_name | shard_count | worker_count | last_update_time |
|-----------|----------------------|-------------|----------|-------------|--------------|------------------|
| `my-app_coordinator` | 20 | kds-stream-prod | my-app | 60 | 3 | 2025-11-19T12:05:00Z |
| `worker-1` | 20 | kds-stream-prod | my-app | 60 | 3 | 2025-11-19T12:05:00Z |
| `worker-2` | **20** | kds-stream-prod | my-app | **60** | 3 | **2025-11-19T12:05:05Z** |
| `worker-3` | 10 | kds-stream-prod | my-app | 30 | 3 | 2025-11-19T10:00:04Z |

**Result**: Worker-2 now uses **max_leases_per_worker = 20**

---

### Step 7: Worker-3 Restarts (10 seconds later)

Time: 2025-11-19T12:05:15Z

**Actions by Worker-3:**
1. Calls `InitializeMaxLeasesPerWorker(ctx)`
2. Table already exists (no-op)
3. Calls `GetShardCount()` → returns **60**
4. Calls `GetWorkerCount()` → returns **3**
5. Calls `GetCoordinatorMetadata()` → returns existing coordinator metadata (max_leases=20)
6. Checks if config changed: `60 == 60 && 3 == 3` → **No change**
7. Uses existing coordinator value: **max_leases = 20**
8. Saves worker-3's own metadata

**DynamoDB Table State After Step 7:**

| worker_id | max_leases_per_worker | stream_name | app_name | shard_count | worker_count | last_update_time |
|-----------|----------------------|-------------|----------|-------------|--------------|------------------|
| `my-app_coordinator` | 20 | kds-stream-prod | my-app | 60 | 3 | 2025-11-19T12:05:00Z |
| `worker-1` | 20 | kds-stream-prod | my-app | 60 | 3 | 2025-11-19T12:05:00Z |
| `worker-2` | 20 | kds-stream-prod | my-app | 60 | 3 | 2025-11-19T12:05:05Z |
| `worker-3` | **20** | kds-stream-prod | my-app | **60** | 3 | **2025-11-19T12:05:15Z** |

**Result**: Worker-3 now uses **max_leases_per_worker = 20**

**System State**: All 3 workers are now running with max_leases=20. Each worker can handle up to 20 shards. Total capacity = 60 leases across 3 workers, which perfectly matches 60 shards!

---

## Edge Case: Multiple Workers Restart Simultaneously

### Step 8: What if Worker-2 and Worker-3 Restart at the Same Time?

Time: 2025-11-19T12:05:05Z (same time)

**Actions by Worker-2:**
1. Gets shard_count=60, worker_count=3
2. Gets coordinator metadata: shard_count=30, worker_count=3, max_leases=10
3. Detects change, calculates new max_leases=20
4. Calls `UpdateCoordinatorMetadata()` with condition: `shard_count=30 AND worker_count=3`
5. **SUCCESS** - Updates coordinator metadata first

**Actions by Worker-3 (parallel):**
1. Gets shard_count=60, worker_count=3
2. Gets coordinator metadata: shard_count=30, worker_count=3, max_leases=10
3. Detects change, calculates new max_leases=20
4. Calls `UpdateCoordinatorMetadata()` with condition: `shard_count=30 AND worker_count=3`
5. **CONDITIONAL CHECK FAILED** - Worker-2 already updated it!
6. Returns nil (not an error) - logs "Another worker already updated coordinator metadata"
7. Re-reads coordinator metadata (now has max_leases=20, shard_count=60)
8. Uses the updated value: **max_leases = 20**

**Result**: Race condition handled gracefully via DynamoDB conditional writes. Both workers end up using the same correct value (max_leases=20).

---

## Summary Table

### Calculation at Each Phase

| Phase | Shards | Workers | Formula | Max Leases Per Worker |
|-------|--------|---------|---------|----------------------|
| Initial | 30 | 3 | `ceil(30/3) = 10` | **10** |
| After Scale | 60 | 3 | `ceil(60/3) = 20` | **20** |

### Key Points

1. **Single Source of Truth**: The `<appName>_coordinator` entry in DynamoDB is the authoritative source for max_leases_per_worker

2. **Race-Safe**: Multiple workers restarting simultaneously won't cause conflicts due to DynamoDB conditional writes

3. **Auto-Detection**: Workers automatically detect when shard count or worker count changes and recalculate

4. **Gradual Update**: Workers that don't restart will continue with old config until they restart

5. **No Manual Intervention**: The system automatically adjusts max_leases_per_worker based on current infrastructure state

6. **80 Lease Limit**: If `ceil(shardCount/workerCount) > 80`, it's capped at 80 to prevent overloading a single worker

---

## Code Flow in kds_record_processor.go

When a worker starts processing KDS records:

```
kdsRecordProcessorFactory() called
  ↓
if cfg.EnableDynamicMaxLeases == true:
  ↓
  leaseManager = NewKDSLeaseManager(ctx, region, stream, app, workerID)
  ↓
  maxLeasesPerWorker = leaseManager.InitializeMaxLeasesPerWorker(ctx)
    ↓
    1. InitializeMetadataTable() - creates table if needed
    2. GetShardCount() - queries Kinesis API
    3. GetWorkerCount() - queries K8s API
    4. GetCoordinatorMetadata() - reads from DynamoDB
    5. If coordinator exists and config changed:
       - CalculateMaxLeasesPerWorker()
       - UpdateCoordinatorMetadata() (conditional write)
    6. If no coordinator:
       - CalculateMaxLeasesPerWorker()
       - TryCreateCoordinatorMetadata() (conditional write)
    7. SaveMetadata() - save this worker's metadata
  ↓
  cfg.MaxLeaseCount = maxLeasesPerWorker
  ↓
recordProcessorFactory created with updated cfg
  ↓
KCL uses cfg.MaxLeaseCount to limit lease acquisition
```

---

## Example Logs

### Worker-1 (First to Detect Change)

```
INFO  Initializing max leases per worker  stream=kds-stream-prod app=my-app worker=worker-1
INFO  Retrieved current system state  currentShardCount=60 currentWorkerCount=3
INFO  Detected configuration change, recalculating max leases per worker  oldShardCount=30 newShardCount=60 oldWorkerCount=3 newWorkerCount=3 oldMaxLeases=10
INFO  Calculated max leases per worker  shardCount=60 workerCount=3 shardsPerWorker=20 maxLeasesPerWorker=20
INFO  Successfully updated coordinator metadata  coordinatorKey=my-app_coordinator newMaxLeasesPerWorker=20
INFO  Successfully initialized dynamic max leases per worker  maxLeasesPerWorker=20
```

### Worker-2 (Reads Updated Value)

```
INFO  Initializing max leases per worker  stream=kds-stream-prod app=my-app worker=worker-2
INFO  Retrieved current system state  currentShardCount=60 currentWorkerCount=3
INFO  Configuration unchanged, using existing coordinator metadata  maxLeasesPerWorker=20 shardCount=60 workerCount=3
INFO  Successfully initialized dynamic max leases per worker  maxLeasesPerWorker=20
```

---

## Testing Recommendations

1. **Unit Tests**: Mock Kinesis and DynamoDB clients to test various scenarios
2. **Integration Tests**: Use LocalStack to simulate real AWS services



