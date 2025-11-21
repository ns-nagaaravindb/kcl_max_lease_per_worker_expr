#!/bin/bash

APPLICATION_NAME="UK-LON3"
AWS_PROFILE="${AWS_PROFILE:-prod}"
REGION="${AWS_REGION:-eu-west-2}"
TABLE_NAME="${APPLICATION_NAME}"
CHECKPOINT_FILE="/tmp/checkpoint_history_${TABLE_NAME}.json"
CHECK_INTERVAL="${CHECK_INTERVAL:-60}"  # seconds between checks

echo "=========================================="
echo "🔍 DynamoDB Checkpoint Progress Monitor"
echo "=========================================="
echo "Application: $APPLICATION_NAME"
echo "AWS Profile: $AWS_PROFILE"
echo "Region: $REGION"
echo "Check Interval: ${CHECK_INTERVAL}s"
echo ""

# Function to get current checkpoints
get_checkpoints() {
    aws dynamodb scan \
        --table-name "$TABLE_NAME" \
        --region "$REGION" \
        --profile "$AWS_PROFILE" \
        --projection-expression "ShardID,Checkpoint,AssignedTo,LeaseCounter" \
        --output json 2>/dev/null
}

# Function to parse checkpoint sequence number
get_sequence_number() {
    local checkpoint="$1"
    if [ "$checkpoint" = "SHARD_END" ] || [ "$checkpoint" = "TRIM_HORIZON" ] || [ -z "$checkpoint" ]; then
        echo "$checkpoint"
    else
        echo "$checkpoint"
    fi
}

# First scan - get baseline
echo "📊 Taking initial checkpoint snapshot..."
CURRENT_SCAN=$(get_checkpoints)

if [ $? -ne 0 ]; then
    echo "❌ Failed to scan DynamoDB table '$TABLE_NAME'"
    echo "   Make sure the table exists and AWS credentials are configured."
    exit 1
fi

TOTAL_SHARDS=$(echo "$CURRENT_SCAN" | jq -r '.Items | length')
echo "   Found $TOTAL_SHARDS shards"
echo ""

# Save current state
echo "$CURRENT_SCAN" | jq '{
    timestamp: now,
    items: [.Items[] | {
        shard: .ShardID.S,
        checkpoint: .Checkpoint.S,
        assignedTo: (.AssignedTo.S // "unassigned"),
        leaseCounter: (.LeaseCounter.N // "0")
    }]
}' > "$CHECKPOINT_FILE"

echo "⏳ Waiting ${CHECK_INTERVAL} seconds before next check..."
echo ""
sleep "$CHECK_INTERVAL"

# Second scan - compare for progress
echo "📊 Taking second checkpoint snapshot..."
CURRENT_SCAN=$(get_checkpoints)

if [ $? -ne 0 ]; then
    echo "❌ Failed to scan DynamoDB table"
    exit 1
fi

# Load previous state
PREVIOUS_STATE=$(cat "$CHECKPOINT_FILE")

echo ""
echo "=========================================="
echo "📈 Checkpoint Progress Analysis"
echo "=========================================="
echo ""

# Analyze each shard - using process substitution to avoid subshell
STALLED_COUNT=0
PROGRESSING_COUNT=0
CLOSED_COUNT=0
UNASSIGNED_COUNT=0

while IFS='|' read -r shard_id current_checkpoint assigned_to lease_counter; do
    
    # Get previous checkpoint for this shard
    previous_checkpoint=$(echo "$PREVIOUS_STATE" | jq -r --arg shard "$shard_id" '.items[] | select(.shard == $shard) | .checkpoint')
    previous_assigned=$(echo "$PREVIOUS_STATE" | jq -r --arg shard "$shard_id" '.items[] | select(.shard == $shard) | .assignedTo')
    
    # Skip if shard is closed
    if [ "$current_checkpoint" = "SHARD_END" ]; then
        CLOSED_COUNT=$((CLOSED_COUNT + 1))
        continue
    fi
    
    # Check if unassigned
    if [ "$assigned_to" = "unassigned" ] || [ -z "$assigned_to" ]; then
        echo "⚠️  UNASSIGNED: $shard_id"
        echo "   Current Checkpoint: $current_checkpoint"
        echo "   Status: Not assigned to any worker"
        echo ""
        UNASSIGNED_COUNT=$((UNASSIGNED_COUNT + 1))
        continue
    fi
    
    # Check if checkpoint has changed
    if [ "$previous_checkpoint" = "$current_checkpoint" ]; then
        # No progress detected
        echo "🔴 STALLED: $shard_id"
        echo "   Assigned To: $assigned_to"
        echo "   Current Checkpoint: $current_checkpoint"
        echo "   Previous Checkpoint: $previous_checkpoint"
        echo "   Lease Counter: $lease_counter"
        echo "   ⚠️  No progress detected in ${CHECK_INTERVAL}s"
        echo ""
        STALLED_COUNT=$((STALLED_COUNT + 1))
    else
        # Progress detected
        echo "✅ PROGRESSING: $shard_id"
        echo "   Assigned To: $assigned_to"
        echo "   Previous Checkpoint: $previous_checkpoint"
        echo "   Current Checkpoint: $current_checkpoint"
        echo "   Status: Making progress"
        echo ""
        PROGRESSING_COUNT=$((PROGRESSING_COUNT + 1))
    fi
done < <(echo "$CURRENT_SCAN" | jq -r '.Items[] | "\(.ShardID.S)|\(.Checkpoint.S)|\(.AssignedTo.S // "unassigned")|\(.LeaseCounter.N // "0")"')

# Update checkpoint file for next run
echo "$CURRENT_SCAN" | jq '{
    timestamp: now,
    items: [.Items[] | {
        shard: .ShardID.S,
        checkpoint: .Checkpoint.S,
        assignedTo: (.AssignedTo.S // "unassigned"),
        leaseCounter: (.LeaseCounter.N // "0")
    }]
}' > "$CHECKPOINT_FILE"

echo "=========================================="
echo "📊 Summary"
echo "=========================================="
echo "Total Shards: $TOTAL_SHARDS"
echo "├─ ✅ Progressing: $PROGRESSING_COUNT"
echo "├─ 🔴 Stalled: $STALLED_COUNT"
echo "├─ ⚠️  Unassigned: $UNASSIGNED_COUNT"
echo "└─ 🔒 Closed (SHARD_END): $CLOSED_COUNT"
echo ""

if [ "$STALLED_COUNT" -gt 0 ]; then
    echo "⚠️  WARNING: $STALLED_COUNT shard(s) are not making progress!"
    echo "💡 Possible reasons:"
    echo "   - Worker is stuck or crashed"
    echo "   - No data in the stream for these shards"
    echo "   - Processing errors preventing checkpoint updates"
    echo ""
elif [ "$UNASSIGNED_COUNT" -gt 0 ]; then
    echo "⚠️  WARNING: $UNASSIGNED_COUNT shard(s) are unassigned!"
    echo "💡 Workers may be at MaxLeasesForWorker limit"
    echo ""
else
    echo "✅ All assigned shards are making progress!"
    echo ""
fi

echo "💾 Checkpoint history saved to: $CHECKPOINT_FILE"
echo "🔄 Run this script again to continue monitoring"
echo ""
