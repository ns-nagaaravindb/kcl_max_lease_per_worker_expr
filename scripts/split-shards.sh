#!/bin/bash

set -e

STREAM_NAME="mohan-experiment-stream"
CONTAINER_NAME="localstack-mohan-experiment"
REGION="us-east-1"
CURRENT_SHARD_COUNT=20
TARGET_SHARD_COUNT=30

echo "=========================================="
echo "🔄 Shard Split: Increasing from $CURRENT_SHARD_COUNT to $TARGET_SHARD_COUNT shards"
echo "=========================================="

echo "📝 Current stream status:"
docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region $REGION \
    --query 'StreamDescription.{StreamName:StreamName,Status:StreamStatus,ShardCount:length(Shards)}' \
    --output table

echo ""
echo "⚠️  Note: LocalStack's update-shard-count may have limitations."
echo "   Using UNIFORM_SCALING to split shards evenly..."
echo ""

# Update shard count (50% increase)
echo "📤 Updating shard count to $TARGET_SHARD_COUNT..."
docker exec $CONTAINER_NAME awslocal kinesis update-shard-count \
    --stream-name "$STREAM_NAME" \
    --target-shard-count $TARGET_SHARD_COUNT \
    --scaling-type UNIFORM_SCALING \
    --region $REGION || {
        echo "⚠️  update-shard-count may not be fully supported by LocalStack"
        echo "   Attempting alternative approach: manual shard splitting..."
        echo ""
        
        # Alternative: Manual shard splitting
        echo "📋 Getting list of open shards to split..."
        SHARD_IDS=$(docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
            --stream-name "$STREAM_NAME" \
            --region $REGION \
            --query 'StreamDescription.Shards[?!EndingSequenceNumber].ShardId' \
            --output text)
        
        SPLIT_COUNT=0
        TARGET_SPLITS=10  # Split 10 shards to go from 20 -> 30
        
        for SHARD_ID in $SHARD_IDS; do
            if [ $SPLIT_COUNT -ge $TARGET_SPLITS ]; then
                break
            fi
            
            echo "   Splitting shard: $SHARD_ID ($((SPLIT_COUNT + 1))/$TARGET_SPLITS)"
            
            # Get the hash key range for this shard
            HASH_RANGE=$(docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
                --stream-name "$STREAM_NAME" \
                --region $REGION \
                --query "StreamDescription.Shards[?ShardId=='$SHARD_ID'].HashKeyRange" \
                --output json)
            
            START_HASH=$(echo $HASH_RANGE | jq -r '.[0].StartingHashKey')
            END_HASH=$(echo $HASH_RANGE | jq -r '.[0].EndingHashKey')
            
            # Calculate midpoint for split
            # Note: This is simplified - production would use proper big integer math
            MID_HASH=$(awk "BEGIN {printf \"%.0f\", ($START_HASH + $END_HASH) / 2}")
            
            # Split the shard
            docker exec $CONTAINER_NAME awslocal kinesis split-shard \
                --stream-name "$STREAM_NAME" \
                --shard-to-split "$SHARD_ID" \
                --new-starting-hash-key "$MID_HASH" \
                --region $REGION || echo "   Failed to split $SHARD_ID"
            
            SPLIT_COUNT=$((SPLIT_COUNT + 1))
            
            # Rate limit to avoid overwhelming LocalStack
            sleep 0.1
        done
        
        echo "✅ Completed $SPLIT_COUNT shard splits"
    }

echo ""
echo "⏳ Waiting for stream to stabilize..."
sleep 15

echo ""
echo "📊 Updated stream status:"
docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region $REGION \
    --query 'StreamDescription.{StreamName:StreamName,Status:StreamStatus,TotalShards:length(Shards),OpenShards:length(Shards[?!EndingSequenceNumber])}' \
    --output table

echo ""
echo "📋 Parent-Child Shard Relationships:"
echo "   (Showing shards with parent/child relationships)"
docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region $REGION \
    --query 'StreamDescription.Shards[?ParentShardId].[ShardId,ParentShardId,SequenceNumberRange.StartingSequenceNumber]' \
    --output table | head -20

echo ""
echo "=========================================="
echo "✅ Shard split completed!"
echo "=========================================="
echo ""
echo "🎯 Next Steps:"
echo "   1. Child shards are now available for processing"
echo "   2. KCL consumers will detect new shards within 5 seconds"
echo "   3. Start additional consumers: make consumer-pod4, make consumer-pod5"
echo "   4. Monitor lease rebalancing: ./scripts/monitor-leases.sh"
echo ""
echo "💡 Expected Behavior:"
echo "   - Child shards start processing immediately (not waiting for parent)"
echo "   - Lease stealing redistributes shards evenly"
echo "   - Target: 6 shards per pod (30 shards / 5 pods)"

