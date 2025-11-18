#!/bin/bash

APPLICATION_NAME="mohan-kcl-consumer"
TABLE_NAME="${APPLICATION_NAME}"
CONTAINER_NAME="localstack-mohan-experiment"
REGION="us-east-1"

echo "=========================================="
echo "📈 Analyzing KCL Rebalancing Performance"
echo "=========================================="
echo ""

# Get lease data
LEASES=$(docker exec $CONTAINER_NAME awslocal dynamodb scan \
    --table-name "$TABLE_NAME" \
    --region $REGION \
    --output json 2>/dev/null)

if [ $? -ne 0 ]; then
    echo "❌ Failed to access lease table"
    exit 1
fi

# Extract metrics
TOTAL_LEASES=$(echo "$LEASES" | jq -r '.Items | length')
ASSIGNED_LEASES=$(echo "$LEASES" | jq -r '.Items[] | select(.AssignedTo != null) | .AssignedTo.S' | wc -l | tr -d ' ')
UNASSIGNED_LEASES=$((TOTAL_LEASES - ASSIGNED_LEASES))

NUM_WORKERS=$(echo "$LEASES" | jq -r '.Items[] | select(.AssignedTo != null) | .AssignedTo.S' | sort -u | wc -l | tr -d ' ')

echo "📊 Overall Metrics:"
echo "   Total Shards (Leases): $TOTAL_LEASES"
echo "   Assigned Leases: $ASSIGNED_LEASES"
echo "   Unassigned Leases: $UNASSIGNED_LEASES"
echo "   Active Workers: $NUM_WORKERS"
echo ""

if [ "$NUM_WORKERS" -gt 0 ]; then
    IDEAL_PER_WORKER=$(awk "BEGIN {printf \"%.2f\", $TOTAL_LEASES / $NUM_WORKERS}")
    
    echo "📊 Distribution Analysis:"
    echo "   Ideal Leases per Worker: $IDEAL_PER_WORKER"
    echo ""
    
    echo "👥 Per-Worker Analysis:"
    echo "$LEASES" | jq -r '.Items[] | select(.AssignedTo != null) | .AssignedTo.S' | sort | uniq -c | sort -rn | while read count worker; do
        DIFF=$(awk "BEGIN {printf \"%.2f\", $count - $IDEAL_PER_WORKER}")
        PERCENT=$(awk "BEGIN {printf \"%.1f\", ($count / $IDEAL_PER_WORKER - 1) * 100}")
        
        if (( $(echo "$DIFF > 5" | bc -l) )); then
            STATUS="⚠️  Overloaded"
        elif (( $(echo "$DIFF < -5" | bc -l) )); then
            STATUS="⚡ Underutilized"
        else
            STATUS="✅ Balanced"
        fi
        
        echo "   $worker:"
        echo "      Leases: $count"
        echo "      Difference from Ideal: $DIFF ($PERCENT%)"
        echo "      Status: $STATUS"
        echo ""
    done
    
    # Calculate standard deviation
    echo "📊 Statistical Analysis:"
    COUNTS=$(echo "$LEASES" | jq -r '.Items[] | select(.AssignedTo != null) | .AssignedTo.S' | sort | uniq -c | awk '{print $1}')
    
    # Calculate mean
    MEAN=$(echo "$COUNTS" | awk '{sum+=$1; count++} END {printf "%.2f", sum/count}')
    
    # Calculate standard deviation
    STD_DEV=$(echo "$COUNTS" | awk -v mean=$MEAN '{sum+=($1-mean)^2; count++} END {printf "%.2f", sqrt(sum/count)}')
    
    # Calculate coefficient of variation
    CV=$(awk "BEGIN {printf \"%.2f\", ($STD_DEV / $MEAN) * 100}")
    
    echo "   Mean: $MEAN"
    echo "   Standard Deviation: $STD_DEV"
    echo "   Coefficient of Variation: $CV%"
    echo ""
    
    if (( $(echo "$CV < 5" | bc -l) )); then
        echo "   ✅ Excellent balance (CV < 5%)"
    elif (( $(echo "$CV < 15" | bc -l) )); then
        echo "   ⚡ Good balance (CV < 15%)"
    elif (( $(echo "$CV < 30" | bc -l) )); then
        echo "   ⚠️  Moderate imbalance (CV < 30%)"
    else
        echo "   ❌ Poor balance (CV >= 30%)"
    fi
fi

echo ""

# Analyze parent-child shard processing
echo "📊 Parent-Child Shard Analysis:"
STREAM_NAME="mohan-experiment-stream"

PARENT_SHARDS=$(docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region $REGION \
    --query 'length(StreamDescription.Shards[?EndingSequenceNumber])' \
    --output text 2>/dev/null)

CHILD_SHARDS=$(docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region $REGION \
    --query 'length(StreamDescription.Shards[?ParentShardId])' \
    --output text 2>/dev/null)

if [ -n "$PARENT_SHARDS" ] && [ -n "$CHILD_SHARDS" ]; then
    echo "   Parent Shards (closed): $PARENT_SHARDS"
    echo "   Child Shards: $CHILD_SHARDS"
    
    if [ "$CHILD_SHARDS" -gt 0 ]; then
        echo "   ✅ Child shards detected - verify they are being processed"
    fi
else
    echo "   ℹ️  No split detected yet"
fi

echo ""
echo "=========================================="
echo "✅ Analysis Complete"
echo "=========================================="

