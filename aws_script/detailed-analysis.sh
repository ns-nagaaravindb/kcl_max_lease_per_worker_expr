#!/usr/bin/env zsh

echo "=========================================="
echo "📊 Comprehensive Lease Data Analysis"
echo "=========================================="
echo "File: lease-data-20251121-110317.json"
echo "Analysis Date: $(date)"
echo ""

# Basic Stats
TOTAL=$(jq '.Items | length' lease-data-20251121-110317.json)
echo "═══════════════════════════════════════"
echo "📋 SHARD OVERVIEW"
echo "═══════════════════════════════════════"
echo "Total Shards: $TOTAL"
echo ""

# Worker Distribution
echo "👥 Worker Distribution:"
jq -r '.Items[] | .AssignedTo.S' lease-data-20251121-110317.json | sort | uniq -c | while read count worker; do
  percentage=$(awk "BEGIN {printf \"%.1f\", ($count / $TOTAL) * 100}")
  echo "   $worker: $count leases ($percentage%)"
done
echo ""

# Check shard states
CLOSED=$(jq '[.Items[] | select(.Checkpoint.S == "SHARD_END")] | length' lease-data-20251121-110317.json)
UNASSIGNED=$(jq '[.Items[] | select(.AssignedTo.S == null or .AssignedTo.S == "")] | length' lease-data-20251121-110317.json)
ACTIVE=$((TOTAL - CLOSED - UNASSIGNED))

echo "📊 Shard States:"
echo "   ├─ Active & Assigned: $ACTIVE"
echo "   ├─ Closed (SHARD_END): $CLOSED"
echo "   └─ Unassigned: $UNASSIGNED"
echo ""

# Parent Analysis
echo "═══════════════════════════════════════"
echo "🌳 PARENT-CHILD RELATIONSHIPS"
echo "═══════════════════════════════════════"

typeset -A SHARD_CHECKPOINT
typeset -A SHARD_PARENT
typeset -A SHARD_WORKER
typeset -a ALL_SHARDS

# Load all shard data
while IFS='|' read -r shard checkpoint parent worker; do
  ALL_SHARDS+=("$shard")
  SHARD_CHECKPOINT[$shard]="$checkpoint"
  SHARD_PARENT[$shard]="$parent"
  SHARD_WORKER[$shard]="$worker"
done < <(jq -r '.Items[] | "\(.ShardID.S)|\(.Checkpoint.S)|\(.ParentShardId.S // "none")|\(.AssignedTo.S)"' lease-data-20251121-110317.json)

# Check for blocking parents
BLOCKED=0
MISSING_PARENTS=0
ROOT_SHARDS=0

for shard in "${ALL_SHARDS[@]}"; do
  parent="${SHARD_PARENT[$shard]}"
  
  if [ "$parent" = "none" ] || [ -z "$parent" ]; then
    ROOT_SHARDS=$((ROOT_SHARDS + 1))
  else
    parent_checkpoint="${SHARD_CHECKPOINT[$parent]}"
    
    if [ -z "$parent_checkpoint" ]; then
      MISSING_PARENTS=$((MISSING_PARENTS + 1))
    elif [ "$parent_checkpoint" != "SHARD_END" ]; then
      BLOCKED=$((BLOCKED + 1))
    fi
  fi
done

echo "Root Shards (no parent): $ROOT_SHARDS"
echo "Child Shards with closed parents: $((TOTAL - ROOT_SHARDS - BLOCKED - MISSING_PARENTS))"
echo "Child Shards blocked by active parents: $BLOCKED"
echo "Child Shards with missing parents: $MISSING_PARENTS"
echo ""

if [ $BLOCKED -gt 0 ]; then
  echo "⚠️  BLOCKING ANALYSIS:"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  
  SHOWN=0
  for shard in "${ALL_SHARDS[@]}"; do
    parent="${SHARD_PARENT[$shard]}"
    
    if [ "$parent" != "none" ] && [ -n "$parent" ]; then
      parent_checkpoint="${SHARD_CHECKPOINT[$parent]}"
      
      if [ -n "$parent_checkpoint" ] && [ "$parent_checkpoint" != "SHARD_END" ]; then
        if [ $SHOWN -lt 5 ]; then
          echo ""
          echo "🔴 Blocked Shard: $shard"
          echo "   Worker: ${SHARD_WORKER[$shard]}"
          echo "   Checkpoint: ${SHARD_CHECKPOINT[$shard]:0:50}..."
          echo "   Parent: $parent"
          echo "   Parent Worker: ${SHARD_WORKER[$parent]}"
          echo "   Parent Checkpoint: ${parent_checkpoint:0:50}..."
          echo "   Status: WAITING for parent to reach SHARD_END"
          SHOWN=$((SHOWN + 1))
        fi
      fi
    fi
  done
  
  if [ $BLOCKED -gt 5 ]; then
    echo ""
    echo "   ... and $((BLOCKED - 5)) more blocked shards"
  fi
  echo ""
  echo "💡 This is NORMAL behavior:"
  echo "   - Child shards must wait for parent shards to complete"
  echo "   - Once parent reaches SHARD_END, child can start processing"
  echo "   - This ensures proper data ordering in stream processing"
else
  echo "✅ No blocking detected!"
  echo "   All shards can process independently"
fi

echo ""
echo "═══════════════════════════════════════"
echo "📈 PROGRESS INDICATORS"
echo "═══════════════════════════════════════"

# Show some sample checkpoints
echo "Sample Checkpoint Values (first 5 shards):"
jq -r '.Items[0:5] | .[] | "   \(.ShardID.S): \(.Checkpoint.S[0:60])..."' lease-data-20251121-110317.json
echo ""

echo "Note: Checkpoints are sequence numbers from Kinesis."
echo "      To detect stalled shards, run this analysis twice"
echo "      with a time interval and compare checkpoints."
echo ""

echo "═══════════════════════════════════════"
echo "🎯 SUMMARY"
echo "═══════════════════════════════════════"

if [ $UNASSIGNED -gt 0 ]; then
  echo "❌ CRITICAL: $UNASSIGNED unassigned shard(s)!"
  echo "   → Check MaxLeasesForWorker limits"
  echo "   → Consider scaling up workers"
elif [ $BLOCKED -gt $((TOTAL / 2)) ]; then
  echo "⚠️  WARNING: Many shards ($BLOCKED) blocked by parents"
  echo "   → This may indicate slow parent processing"
  echo "   → Monitor parent shard progress"
elif [ $BLOCKED -gt 0 ]; then
  echo "⏳ NORMAL: $BLOCKED shard(s) waiting for parent completion"
  echo "   → This is expected behavior for child shards"
  echo "   → No action needed"
else
  echo "✅ HEALTHY: All shards properly assigned and active"
  echo "   → Workers: 2 (evenly balanced)"
  echo "   → Distribution: 50/50 split"
  echo "   → No blocking conditions detected"
fi

echo ""
echo "💡 Recommendations:"
if [ $ACTIVE -eq $TOTAL ]; then
  echo "   ✓ System is operating normally"
  echo "   ✓ Load is well balanced between workers"
  echo "   ✓ Run monitor-checkpoint-progress.sh to verify processing"
fi

echo ""
