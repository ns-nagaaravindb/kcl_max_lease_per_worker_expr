#!/bin/bash

APPLICATION_NAME="${APPLICATION_NAME:-UK-LON3}" 
# APPLICATION_NAME="UK-LON3"
AWS_PROFILE="${AWS_PROFILE:-prod}"
# AWS_PROFILE="${AWS_PROFILE:-prod}"
REGION="${AWS_REGION:-eu-west-2}"
# REGION="${AWS_REGION:-eu-west-2}"
TABLE_NAME="${APPLICATION_NAME}"


echo "=========================================="
echo "📊 Current Lease Distribution Snapshot"
echo "=========================================="
echo "Using AWS Profile: $AWS_PROFILE"
echo "Region: $REGION"
echo ""

# Get all lease items
LEASES=$(aws dynamodb scan \
    --table-name "$TABLE_NAME" \
    --region $REGION \
    --profile $AWS_PROFILE \
    --output json 2>/dev/null)

if [ $? -ne 0 ]; then
    echo "❌ Failed to scan lease table. Make sure consumers are running."
    exit 1
fi

# Count various shard states
TOTAL_LEASES=$(echo "$LEASES" | jq -r '.Items | length')
ACTIVE_ASSIGNED=$(echo "$LEASES" | jq -r '[.Items[] | select(.AssignedTo != null and .AssignedTo.S != "")] | length')
SHARD_END_COUNT=$(echo "$LEASES" | jq -r '[.Items[] | select(.Checkpoint.S == "SHARD_END")] | length')
UNASSIGNED_ACTIVE=$(echo "$LEASES" | jq -r '[.Items[] | select((.AssignedTo == null or .AssignedTo.S == "") and (.Checkpoint.S != "SHARD_END"))] | length')

echo "📋 Shard Overview:"
echo "   Total Shards: $TOTAL_LEASES"
echo "   ├─ Active & Assigned: $ACTIVE_ASSIGNED"
echo "   ├─ Closed (SHARD_END): $SHARD_END_COUNT"
echo "   └─ Unassigned (Active): $UNASSIGNED_ACTIVE"
echo ""

# Count leases per worker
echo "👥 Leases per Worker:"
echo ""
echo "$LEASES" | jq -r '.Items[] | select(.AssignedTo != null and .AssignedTo.S != "") | .AssignedTo.S' | sort | uniq -c | sort -rn | while read count worker; do
    echo "   $worker: $count leases"
done

echo ""

# Show all shard assignments grouped by worker
echo "📋 All Shard Assignments (grouped by worker):"
echo "$LEASES" | jq -r '.Items[] | 
    if .Checkpoint.S == "SHARD_END" then "CLOSED_SHARDS\t\(.ShardID.S) [SHARD_END]"
    elif (.AssignedTo == null or .AssignedTo.S == "") then "UNASSIGNED_ACTIVE\t\(.ShardID.S)"
    else "\(.AssignedTo.S)\t\(.ShardID.S)"
    end' | sort | awk -F'\t' '
    {
        if ($1 != prev_worker) {
            if (NR > 1) printf "\n";
            if ($1 == "CLOSED_SHARDS") {
                printf "   🔒 Closed Shards (SHARD_END):\n";
            } else if ($1 == "UNASSIGNED_ACTIVE") {
                printf "   ⚠️  Unassigned Active Shards:\n";
            } else {
                printf "   %s:\n", $1;
            }
            prev_worker = $1;
        }
        printf "      - %s\n", $2;
    }
'

echo ""

# Calculate distribution metrics
WORKERS=$(echo "$LEASES" | jq -r '.Items[] | select(.AssignedTo != null and .AssignedTo.S != "") | .AssignedTo.S' | sort -u)
NUM_WORKERS=$(echo "$WORKERS" | wc -l | tr -d ' ')

if [ "$NUM_WORKERS" -gt 0 ]; then
    AVG_LEASES=$(awk "BEGIN {printf \"%.2f\", $ACTIVE_ASSIGNED / $NUM_WORKERS}")
    IDEAL_LEASES=$(awk "BEGIN {printf \"%.0f\", $ACTIVE_ASSIGNED / $NUM_WORKERS}")
    
    echo "📊 Distribution Metrics:"
    echo "   Active Workers: $NUM_WORKERS"
    echo "   Active Shards per Worker (avg): $AVG_LEASES"
    echo "   Ideal for Even Distribution: $IDEAL_LEASES leases/worker"
    
    # Check for imbalance
    MAX_LEASES=$(echo "$LEASES" | jq -r '.Items[] | select(.AssignedTo != null and .AssignedTo.S != "") | .AssignedTo.S' | sort | uniq -c | sort -rn | head -1 | awk '{print $1}')
    MIN_LEASES=$(echo "$LEASES" | jq -r '.Items[] | select(.AssignedTo != null and .AssignedTo.S != "") | .AssignedTo.S' | sort | uniq -c | sort -rn | tail -1 | awk '{print $1}')
    IMBALANCE=$((MAX_LEASES - MIN_LEASES))
    
    echo "   Max Leases (one worker): $MAX_LEASES"
    echo "   Min Leases (one worker): $MIN_LEASES"
    echo "   Imbalance: $IMBALANCE"
    echo ""
    
    if [ "$IMBALANCE" -gt 10 ]; then
        echo "   ⚠️  Status: Significant imbalance detected!"
        echo "   💡 Tip: Lease stealing should redistribute within 30-60 seconds"
    elif [ "$IMBALANCE" -gt 5 ]; then
        echo "   ⚡ Status: Rebalancing in progress..."
    else
        echo "   ✅ Status: Well balanced!"
    fi
    
    # Warning for unassigned active shards
    if [ "$UNASSIGNED_ACTIVE" -gt 0 ]; then
        echo ""
        echo "   ⚠️  WARNING: $UNASSIGNED_ACTIVE active shard(s) not assigned to any worker!"
        echo "   💡 Tip: Workers may be at MaxLeasesForWorker limit or not running"
    fi
fi

echo ""
echo "=========================================="

