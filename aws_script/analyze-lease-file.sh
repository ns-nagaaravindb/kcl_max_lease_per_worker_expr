#!/usr/bin/env zsh

# Analyze exported lease data file for deadlocks and stalled shards
INPUT_FILE="$1"

if [ -z "$INPUT_FILE" ]; then
    echo "Usage: $0 <lease-data-file.json>"
    echo ""
    echo "Example: $0 lease-data-20251121-110317.json"
    exit 1
fi

if [ ! -f "$INPUT_FILE" ]; then
    echo "❌ File not found: $INPUT_FILE"
    exit 1
fi

echo "=========================================="
echo "🔍 Lease Data Analysis - Deadlock Detection"
echo "=========================================="
echo "Analyzing file: $INPUT_FILE"
echo ""

# Extract data into associative arrays
typeset -A SHARD_CHECKPOINT
typeset -A SHARD_WORKER
typeset -A SHARD_PARENT
typeset -A SHARD_TIMEOUT
typeset -a ALL_SHARDS

# Parse the JSON file - using arrays to avoid subshell issues
# Read all data into arrays at once
typeset -a shard_ids
typeset -a checkpoints
typeset -a assigned_tos
typeset -a parent_ids
typeset -a lease_timeouts

# Use mapfile/readarray equivalent in zsh
shard_ids=(${(f)"$(jq -r '.Items[] | .ShardID.S' "$INPUT_FILE")"})
checkpoints=(${(f)"$(jq -r '.Items[] | .Checkpoint.S' "$INPUT_FILE")"})
assigned_tos=(${(f)"$(jq -r '.Items[] | .AssignedTo.S // "unassigned"' "$INPUT_FILE")"})
parent_ids=(${(f)"$(jq -r '.Items[] | .ParentShardId.S // "none"' "$INPUT_FILE")"})
lease_timeouts=(${(f)"$(jq -r '.Items[] | .LeaseTimeout.S // "none"' "$INPUT_FILE")"})

# Populate the associative arrays - NOTE: Do NOT quote the key variables in zsh!
for i in {1..${#shard_ids[@]}}; do
    shard="${shard_ids[$i]}"
    ALL_SHARDS+=("$shard")
    SHARD_CHECKPOINT[$shard]="${checkpoints[$i]}"
    SHARD_WORKER[$shard]="${assigned_tos[$i]}"
    SHARD_PARENT[$shard]="${parent_ids[$i]}"
    SHARD_TIMEOUT[$shard]="${lease_timeouts[$i]}"
done

TOTAL_SHARDS=${#ALL_SHARDS[@]}
echo "📊 Total Shards: $TOTAL_SHARDS"
echo ""

# Analyze shard states
CLOSED_SHARDS=0
ACTIVE_SHARDS=0
UNASSIGNED_SHARDS=0

for shard in "${ALL_SHARDS[@]}"; do
    checkpoint="${SHARD_CHECKPOINT[$shard]}"
    worker="${SHARD_WORKER[$shard]}"
    
    if [ "$checkpoint" = "SHARD_END" ]; then
        CLOSED_SHARDS=$((CLOSED_SHARDS + 1))
    elif [ "$worker" = "unassigned" ] || [ -z "$worker" ]; then
        UNASSIGNED_SHARDS=$((UNASSIGNED_SHARDS + 1))
    else
        ACTIVE_SHARDS=$((ACTIVE_SHARDS + 1))
    fi
done

echo "📋 Shard Status Summary:"
echo "   ├─ Active & Assigned: $ACTIVE_SHARDS"
echo "   ├─ Closed (SHARD_END): $CLOSED_SHARDS"
echo "   └─ Unassigned: $UNASSIGNED_SHARDS"
echo ""

# Worker distribution
echo "👥 Worker Distribution:"
typeset -A WORKER_COUNT
for shard in "${ALL_SHARDS[@]}"; do
    worker="${SHARD_WORKER[$shard]}"
    checkpoint="${SHARD_CHECKPOINT[$shard]}"
    
    if [ "$worker" != "unassigned" ] && [ -n "$worker" ] && [ "$checkpoint" != "SHARD_END" ]; then
        WORKER_COUNT[$worker]=$(( ${WORKER_COUNT[$worker]:-0} + 1 ))
    fi
done

for worker in "${(@k)WORKER_COUNT}"; do
    echo "   $worker: ${WORKER_COUNT[$worker]} leases"
done
echo ""

# Analyze parent-child relationships for potential deadlocks
echo "=========================================="
echo "🔍 Deadlock Analysis"
echo "=========================================="
echo ""

POTENTIAL_DEADLOCKS=0
BLOCKED_BY_PARENT=0
PARENT_CHILD_ISSUES=0

# Check each shard to see if it's blocked by parent
for shard in "${ALL_SHARDS[@]}"; do
    checkpoint="${SHARD_CHECKPOINT[$shard]}"
    worker="${SHARD_WORKER[$shard]}"
    parent="${SHARD_PARENT[$shard]}"
    
    # Skip if shard is closed or has no parent
    if [ "$checkpoint" = "SHARD_END" ] || [ "$parent" = "none" ] || [ -z "$parent" ]; then
        continue
    fi
    
    # Check if parent exists in our data
    if [ -z "${SHARD_CHECKPOINT[$parent]}" ]; then
        echo "⚠️  WARNING: Shard $shard references missing parent: $parent"
        PARENT_CHILD_ISSUES=$((PARENT_CHILD_ISSUES + 1))
        continue
    fi
    
    parent_checkpoint="${SHARD_CHECKPOINT[$parent]}"
    parent_worker="${SHARD_WORKER[$parent]}"
    
    # Check if parent is not closed (potential blocking condition)
    if [ "$parent_checkpoint" != "SHARD_END" ]; then
        echo "🔴 BLOCKED: Shard $shard"
        echo "   Child Status: $checkpoint"
        echo "   Child Worker: $worker"
        echo "   Parent: $parent"
        echo "   Parent Status: $parent_checkpoint"
        echo "   Parent Worker: $parent_worker"
        
        BLOCKED_BY_PARENT=$((BLOCKED_BY_PARENT + 1))
        
        # Check for deadlock conditions
        if [ "$parent_worker" = "unassigned" ] || [ -z "$parent_worker" ]; then
            echo "   ❌ DEADLOCK: Parent is UNASSIGNED!"
            echo "      → Parent shard cannot be processed (likely MaxLeasesForWorker limit)"
            POTENTIAL_DEADLOCKS=$((POTENTIAL_DEADLOCKS + 1))
        elif [ "$checkpoint" = "TRIM_HORIZON" ] || [ "$checkpoint" = "" ]; then
            echo "   ⏳ WAITING: Child hasn't started (parent must close first)"
        else
            echo "   ⏳ WAITING: Child is waiting for parent to reach SHARD_END"
        fi
        echo ""
    fi
done

# Analyze lease timeouts (detect stale leases)
echo "=========================================="
echo "⏰ Lease Timeout Analysis"
echo "=========================================="
echo ""

CURRENT_TIME=$(date -u +%s 2>/dev/null || date +%s)
STALE_LEASES=0
EXPIRED_LEASES=0

for shard in "${ALL_SHARDS[@]}"; do
    timeout="${SHARD_TIMEOUT[$shard]}"
    worker="${SHARD_WORKER[$shard]}"
    checkpoint="${SHARD_CHECKPOINT[$shard]}"
    
    if [ "$timeout" = "none" ] || [ -z "$timeout" ] || [ "$checkpoint" = "SHARD_END" ]; then
        continue
    fi
    
    # Convert timeout to timestamp (handle both GNU and BSD date)
    timeout_ts=$(date -u -j -f "%Y-%m-%dT%H:%M:%S" "${timeout:0:19}" +%s 2>/dev/null || date -u -d "${timeout:0:19}" +%s 2>/dev/null)
    
    if [ -n "$timeout_ts" ]; then
        time_diff=$((CURRENT_TIME - timeout_ts))
        
        if [ $time_diff -gt 3600 ]; then
            echo "⚠️  STALE: Shard $shard"
            echo "   Worker: $worker"
            echo "   Lease expired: $(($time_diff / 3600))h $(($time_diff % 3600 / 60))m ago"
            echo "   Last timeout: $timeout"
            echo ""
            STALE_LEASES=$((STALE_LEASES + 1))
            
            if [ $time_diff -gt 86400 ]; then
                EXPIRED_LEASES=$((EXPIRED_LEASES + 1))
            fi
        fi
    fi
done

if [ $STALE_LEASES -eq 0 ]; then
    echo "✅ No stale leases detected (all leases are recent)"
    echo ""
fi

# Summary
echo "=========================================="
echo "📊 Analysis Summary"
echo "=========================================="
echo ""
echo "Total Shards Analyzed: $TOTAL_SHARDS"
echo "├─ Active & Processing: $ACTIVE_SHARDS"
echo "├─ Closed (SHARD_END): $CLOSED_SHARDS"
echo "└─ Unassigned: $UNASSIGNED_SHARDS"
echo ""
echo "Blocking Issues:"
echo "├─ Shards blocked by parent: $BLOCKED_BY_PARENT"
echo "├─ Potential deadlocks: $POTENTIAL_DEADLOCKS"
echo "└─ Parent-child issues: $PARENT_CHILD_ISSUES"
echo ""
echo "Lease Health:"
echo "├─ Stale leases (>1h old): $STALE_LEASES"
echo "└─ Very stale (>24h old): $EXPIRED_LEASES"
echo ""

# Final recommendations
if [ $POTENTIAL_DEADLOCKS -gt 0 ]; then
    echo "❌ CRITICAL: $POTENTIAL_DEADLOCKS deadlock(s) detected!"
    echo ""
    echo "🚨 Immediate Actions Required:"
    echo "   1. Check MaxLeasesForWorker configuration"
    echo "   2. Scale up worker count to handle all shards"
    echo "   3. Investigate why parent shards are unassigned"
    echo "   4. Consider restarting workers to force lease rebalancing"
elif [ $BLOCKED_BY_PARENT -gt 0 ]; then
    echo "⏳ WAITING: $BLOCKED_BY_PARENT shard(s) blocked by parent shards"
    echo ""
    echo "💡 Recommendations:"
    echo "   1. This is normal - child shards wait for parents to complete"
    echo "   2. Monitor parent shard progress"
    echo "   3. Ensure workers are actively processing parent shards"
elif [ $STALE_LEASES -gt 0 ]; then
    echo "⚠️  WARNING: $STALE_LEASES stale lease(s) detected"
    echo ""
    echo "💡 Recommendations:"
    echo "   1. Check if workers are still running"
    echo "   2. Verify worker health and logs"
    echo "   3. Consider restarting affected workers"
else
    echo "✅ No issues detected!"
    echo ""
    echo "💡 System appears healthy:"
    echo "   - All shards properly assigned"
    echo "   - No deadlocks detected"
    echo "   - Lease timeouts are current"
fi

echo ""
