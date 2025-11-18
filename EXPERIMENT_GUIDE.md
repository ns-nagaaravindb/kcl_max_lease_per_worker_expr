# Experiment Execution Guide

## Overview

This guide walks through executing the complete KCL rebalancing experiment step-by-step.

## Prerequisites

- Docker and Docker Compose installed
- Go 1.20+ installed
- `awslocal` CLI (part of LocalStack)
- `jq` for JSON processing
- Multiple terminal windows (8 recommended)

## Experiment Phases

### Phase 1: Initial Setup (20 shards, 3 consumers)

**Expected Duration:** 5-10 minutes

**Terminal 1 - Setup & Monitoring:**

```bash
cd expr_mohan

# Build the binaries
make build

# Start LocalStack with 20 shards
make start

# Verify stream creation
make verify
```

**Expected Output:**
```
✅ Stream has 20 shards
   Total Shards:  20
   Open Shards:   20
   Closed Shards: 0
```

**Terminal 2 - Consumer Pod 1:**

```bash
cd expr_mohan
make consumer-pod1
```

**Terminal 3 - Consumer Pod 2:**

```bash
cd expr_mohan
make consumer-pod2
```

**Terminal 4 - Consumer Pod 3:**

```bash
cd expr_mohan
make consumer-pod3
```

Wait 30 seconds for lease distribution to stabilize.

**Terminal 5 - Monitor Leases:**

```bash
cd expr_mohan
make monitor
```

**Expected Output:**
```
📋 Total Leases: 20

👥 Leases per Worker:
   consumer-pod-1: 7 leases
   consumer-pod-2: 7 leases
   consumer-pod-3: 6 leases

   ✅ Status: Well balanced!
```

**Terminal 6 - Producer:**

```bash
cd expr_mohan
make producer
```

**Verification Checkpoint 1:**

```bash
# In Terminal 1 or a new terminal:
make view-dist
```

Expected lease distribution: ~6-7 leases per consumer (20/3 ≈ 7)

**Success Criteria:**
- All 20 shards assigned
- Each consumer has 6-7 leases
- Imbalance < 2 leases
- Producer sending data successfully

---

### Phase 2: Shard Split (20 → 30 shards)

**Expected Duration:** 2-5 minutes

**Terminal 1 - Execute Split:**

```bash
make split
```

This will:
- Split 10 shards to create 10 new child shards
- Close parent shards
- Create parent-child relationships

**Expected Output:**
```
✅ Completed 10 shard splits

📊 Updated stream status:
   Total Shards: 30
   Open Shards: 20
   Closed Shards: 10
```

**Observe in Terminal 5 (Monitor):**

You should see:
1. Total leases increase from 20 → 30
2. Consumers start detecting and acquiring new child shards
3. Lease counts per consumer temporarily increase

**Time 0s (Before Split):**
```
Pod1: 7, Pod2: 7, Pod3: 6
Total: 20 shards
```

**Time +30s (During Split):**
```
Pod1: 10+, Pod2: 10+, Pod3: 10+
Total: 30+ (includes closed parents)
```

**Time +60s (After Split Stabilizes):**
```
Pod1: 10, Pod2: 10, Pod3: 10
Total: 30 shards
```

**Verification Checkpoint 2:**

```bash
make analyze
```

**Success Criteria:**
- Stream has 30 total shards (20 open, 10 closed)
- Child shards are being processed (check consumer logs)
- No waiting for parent shard completion
- All consumers still processing data

---

### Phase 3: Consumer Scale-Up (3 → 5 pods)

**Expected Duration:** 3-5 minutes

**Terminal 7 - Consumer Pod 4:**

```bash
cd expr_mohan
make consumer-pod4
```

**Terminal 8 - Consumer Pod 5:**

```bash
cd expr_mohan
make consumer-pod5
```

**Observe in Terminal 5 (Monitor):**

Watch the rebalancing happen automatically:

**Time 0s (5 pods started):**
```
Pod1: 10, Pod2: 10, Pod3: 10, Pod4: 0, Pod5: 0
Imbalance: 10
⚠️  Significant imbalance detected!
```

**Time +30s (Lease stealing begins):**
```
Pod1: 8, Pod2: 9, Pod3: 9, Pod4: 2, Pod5: 2
Imbalance: 7
⚡ Rebalancing in progress...
```

**Time +60s (Approaching balance):**
```
Pod1: 7, Pod2: 7, Pod3: 7, Pod4: 5, Pod5: 4
Imbalance: 3
⚡ Rebalancing in progress...
```

**Time +120s (Balanced):**
```
Pod1: 6, Pod2: 6, Pod3: 6, Pod4: 6, Pod5: 6
Imbalance: 0
✅ Well balanced!
```

**Verification Checkpoint 3:**

```bash
make analyze
```

**Expected Output:**
```
📊 Overall Metrics:
   Total Shards (Leases): 30
   Assigned Leases: 30
   Active Workers: 5

   Ideal Leases per Worker: 6.00

👥 Per-Worker Analysis:
   consumer-pod-1: 6 leases ✅ Balanced
   consumer-pod-2: 6 leases ✅ Balanced
   consumer-pod-3: 6 leases ✅ Balanced
   consumer-pod-4: 6 leases ✅ Balanced
   consumer-pod-5: 6 leases ✅ Balanced

   ✅ Excellent balance (CV < 5%)
```

**Success Criteria:**
- All 5 consumers running
- Each consumer has exactly 6 leases (30/5)
- Imbalance ≤ 1 leases
- MaxLeasesForWorker = 6 enforced
- All consumers processing data

---

## Key Observations

### 1. Lease Stealing Algorithm

The algorithm `MaxLeasesForWorker = min(10, NumShards/NumPODs)` is enforced:

- **3 pods, 20 shards:** max = min(10, 20/3) = 6
- **3 pods, 30 shards:** max = min(10, 30/3) = 10
- **5 pods, 30 shards:** max = min(10, 30/5) = 6

### 2. Parent/Child Shard Processing

With `ProcessParentShardBeforeChildren = false`:

- Child shards start processing **immediately** after split
- No waiting for parent shard to complete
- Parent and child can be processed **concurrently**
- Verify in consumer logs: Look for child shard IDs being initialized

### 3. Rebalancing Timeline

Typical timeline for rebalancing:

| Event | Expected Time |
|-------|---------------|
| New consumer starts | T+0s |
| Shard sync detects imbalance | T+5s |
| First lease steal attempt | T+10s |
| Continuous stealing | T+10s to T+120s |
| Convergence to balance | T+120s |

### 4. DynamoDB Lease Table

Monitor the lease table:

```bash
# View raw lease data
make view-dist

# Export for analysis
cd scripts && ./export-lease-data.sh
```

Lease table structure:
```json
{
  "leaseKey": "shardId-000000000123",
  "leaseOwner": "consumer-pod-3",
  "leaseCounter": 5,
  "checkpointSequenceNumber": "49590338...",
  "ownerSwitchesSinceCheckpoint": 2
}
```

---

## Troubleshooting

### Issue: Leases not rebalancing

**Diagnosis:**
```bash
make analyze
```

Look for:
- `Active Workers` count
- `MaxLeasesForWorker` setting
- Consumer logs for lease stealing activity

**Solution:**
- Verify `enable_lease_stealing: true` in config
- Check `max_leases_for_worker` is set correctly
- Ensure `lease_stealing_interval_millis` is not too high

### Issue: Child shards waiting for parent

**Diagnosis:**
```bash
# Check if child shards are in lease table
make view-dist | grep "child"

# Check consumer logs
grep "child" consumer-pod*.log
```

**Solution:**
- Verify `process_parent_shard_before_children: false`
- Check if child shards have correct parent references
- Ensure consumers are using enhanced_consumer.go

### Issue: Shards not splitting

**Diagnosis:**
```bash
make verify
```

Look for `Open Shards` count.

**Solution:**
- LocalStack may have limitations with large shard counts
- Try reducing initial shard count or split count
- Use manual split script approach

### Issue: Consumers not starting

**Diagnosis:**
```bash
# Check dependencies
go mod tidy

# Verify config files
cat config/config-pod*.yaml
```

**Solution:**
- Run `go mod tidy` in consumer directory
- Verify AWS endpoint is reachable
- Check LocalStack is running

---

## Data Collection

### Metrics to Collect

1. **Lease Distribution Timeline:**
   ```bash
   # Run this every 10 seconds during experiment
   while true; do
     echo "=== $(date) ===" >> lease-timeline.log
     make view-dist >> lease-timeline.log
     sleep 10
   done
   ```

2. **Consumer Processing Rates:**
   - Monitor consumer logs for "Rate: X rec/s" messages
   - Expected rate: 5-10 records/sec per consumer

3. **Rebalancing Convergence Time:**
   - Start timer when Pod 4 & 5 start
   - Stop timer when imbalance < 5
   - Expected: 60-120 seconds

4. **Parent/Child Processing:**
   ```bash
   # Check if children start immediately
   grep "child" consumer/logs/*.log | grep "Initializing"
   ```

---

## Clean Up

```bash
# Stop all consumers (Ctrl+C in each terminal)

# Stop producer (Ctrl+C)

# Stop LocalStack
make stop

# Clean all artifacts
make clean
```

---



## Expected Results Summary

| Metric | Initial (3 pods, 20 shards) | After Split (3 pods, 30 shards) | After Scale (5 pods, 30 shards) |
|--------|------------------------------|-----------------------------------|-----------------------------------|
| Total Shards | 20 | 30 | 30 |
| Leases per Pod | 6-7 | ~10 | 6 |
| MaxLeasesForWorker | 6 | 10 | 6 |
| Imbalance | 0-1 | 0-2 | 0-1 |
| Rebalancing Time | N/A | N/A | 60-120s |
| Child Shards Processing | N/A | Immediate | Immediate |

---

## Conclusion

This experiment demonstrates:

1. ✅ **Lease Stealing:** Automatic rebalancing with `EnableLeaseStealing = true`
2. ✅ **Max Leases Algorithm:** `min(10, NumShards/NumPODs)` enforced
3. ✅ **Dynamic Scaling:** Smooth scale from 3 → 5 consumers
4. ✅ **Shard Splitting:** 50% increase (20 → 30 shards)
5. ✅ **Parent/Child Processing:** Children start immediately

The experiment validates KCL's ability to handle dynamic shard and consumer changes while maintaining even distribution and high throughput.

