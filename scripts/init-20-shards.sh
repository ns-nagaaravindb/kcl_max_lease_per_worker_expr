#!/bin/bash

set -e

STREAM_NAME="mohan-experiment-stream"
INITIAL_SHARD_COUNT=20

echo "=========================================="
echo "🚀 Initializing Kinesis Stream with 20 Shards"
echo "=========================================="

echo "⏳ Waiting for LocalStack to be ready..."
sleep 5

echo "📝 Creating stream: $STREAM_NAME with $INITIAL_SHARD_COUNT shards..."
awslocal kinesis create-stream \
    --stream-name "$STREAM_NAME" \
    --shard-count $INITIAL_SHARD_COUNT \
    --region us-east-1

echo "⏳ Waiting for stream to become active (this may take a while)..."
sleep 10

# Wait for stream to be active
MAX_RETRIES=30
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    STATUS=$(awslocal kinesis describe-stream \
        --stream-name "$STREAM_NAME" \
        --region us-east-1 \
        --query 'StreamDescription.StreamStatus' \
        --output text)
    
    echo "   Stream Status: $STATUS (attempt $((RETRY_COUNT + 1))/$MAX_RETRIES)"
    
    if [ "$STATUS" = "ACTIVE" ]; then
        echo "✅ Stream is ACTIVE!"
        break
    fi
    
    RETRY_COUNT=$((RETRY_COUNT + 1))
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "❌ Stream did not become active within expected time"
    exit 1
fi

echo ""
echo "📊 Stream Details:"
awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region us-east-1 \
    --query 'StreamDescription.{StreamName:StreamName,Status:StreamStatus,ShardCount:length(Shards)}' \
    --output table

echo ""
echo "📋 First 10 Shards:"
awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region us-east-1 \
    --query 'StreamDescription.Shards[:10].[ShardId,HashKeyRange.StartingHashKey,HashKeyRange.EndingHashKey]' \
    --output table

echo ""
echo "=========================================="
echo "✅ Stream initialization completed!"
echo "=========================================="
echo ""
echo "🎯 Next Steps:"
echo "   1. Start the producer: make producer"
echo "   2. Start 3 consumers: make consumer-pod1, make consumer-pod2, make consumer-pod3"
echo "   3. Monitor leases: ./scripts/monitor-leases.sh"
echo "   4. Split shards: ./scripts/split-shards.sh"

