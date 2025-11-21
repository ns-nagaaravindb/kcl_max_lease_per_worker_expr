#!/bin/bash

# Check if stream name is provided
if [ -z "$1" ]; then
    echo "❌ Error: Stream name is required"
    echo ""
    echo "Usage: $0 <stream_name>"
    echo ""
    echo "Example: $0 my-kinesis-stream"
    echo ""
    echo "Environment variables (optional):"
    echo "  AWS_PROFILE  - AWS profile to use (default: prod)"
    echo "  AWS_REGION   - AWS region (default: eu-west-2)"
    exit 1
fi

STREAM_NAME="$1"
AWS_PROFILE="${AWS_PROFILE:-prod}"
REGION="${AWS_REGION:-eu-west-2}"
OUTPUT_FILE="kds-shards-${STREAM_NAME}-$(date +%Y%m%d-%H%M%S).json"

echo "📤 Listing all shards from Kinesis Data Stream: $STREAM_NAME"
echo "Using AWS Profile: $AWS_PROFILE"
echo "Region: $REGION"
echo ""

# List all shards from the stream
aws kinesis list-shards \
    --stream-name "$STREAM_NAME" \
    --region $REGION \
    --profile $AWS_PROFILE \
    --output json > "$OUTPUT_FILE" 2>/dev/null

if [ $? -eq 0 ]; then
    echo "✅ Shards data exported to: $OUTPUT_FILE"
    echo ""
    
    # Show summary
    TOTAL_SHARDS=$(jq '.Shards | length' "$OUTPUT_FILE")
    echo "📊 Summary:"
    echo "   Total Shards: $TOTAL_SHARDS"
    echo ""
    
    # Show file size
    FILE_SIZE=$(ls -lh "$OUTPUT_FILE" | awk '{print $5}')
    echo "   File Size: $FILE_SIZE"
    echo ""
    
    # Show breakdown by shard state
    OPEN_SHARDS=$(jq '[.Shards[] | select(.SequenceNumberRange.EndingSequenceNumber == null)] | length' "$OUTPUT_FILE")
    CLOSED_SHARDS=$(jq '[.Shards[] | select(.SequenceNumberRange.EndingSequenceNumber != null)] | length' "$OUTPUT_FILE")
    
    # Count parent shards (shards that have been split)
    PARENT_SHARDS=$(jq '[.Shards[] | select(.ParentShardId != null)] | length' "$OUTPUT_FILE")
    
    echo "   Breakdown:"
    echo "   ├─ Open Shards: $OPEN_SHARDS"
    echo "   ├─ Closed Shards: $CLOSED_SHARDS"
    echo "   └─ Parent Shards (split): $PARENT_SHARDS"
    echo ""
    
    # Show shard IDs (first 10)
    echo "   Sample Shard IDs (first 10):"
    jq -r '.Shards[0:10] | .[] | "   ├─ \(.ShardId)"' "$OUTPUT_FILE"
    if [ $TOTAL_SHARDS -gt 10 ]; then
        echo "   └─ ... and $((TOTAL_SHARDS - 10)) more"
    fi
else
    echo "❌ Failed to list shards from stream '$STREAM_NAME'"
    echo "   Make sure the stream exists and AWS credentials are configured."
    exit 1
fi

