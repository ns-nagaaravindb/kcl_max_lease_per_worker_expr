#!/bin/bash

STREAM_NAME="mohan-experiment-stream"
CONTAINER_NAME="localstack-mohan-experiment"
REGION="us-east-1"

echo "=========================================="
echo "🔍 Verifying Kinesis Stream"
echo "=========================================="

echo ""
echo "📊 Stream Status:"
docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region $REGION \
    --query 'StreamDescription.{StreamName:StreamName,Status:StreamStatus,TotalShards:length(Shards),OpenShards:length(Shards[?!EndingSequenceNumber]),ClosedShards:length(Shards[?EndingSequenceNumber])}' \
    --output table

echo ""
echo "📋 Shard Distribution:"
TOTAL_SHARDS=$(docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region $REGION \
    --query 'length(StreamDescription.Shards)' \
    --output text)

OPEN_SHARDS=$(docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region $REGION \
    --query 'length(StreamDescription.Shards[?!EndingSequenceNumber])' \
    --output text)

CLOSED_SHARDS=$(docker exec $CONTAINER_NAME awslocal kinesis describe-stream \
    --stream-name "$STREAM_NAME" \
    --region $REGION \
    --query 'length(StreamDescription.Shards[?EndingSequenceNumber])' \
    --output text)

echo "   Total Shards:  $TOTAL_SHARDS"
echo "   Open Shards:   $OPEN_SHARDS"
echo "   Closed Shards: $CLOSED_SHARDS"

echo ""
echo "✅ Stream verification complete!"

