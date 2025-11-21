#!/bin/bash

APPLICATION_NAME="${APPLICATION_NAME:-UK-LON3}"
AWS_PROFILE="${AWS_PROFILE:-prod}"
REGION="${AWS_REGION:-eu-west-2}"
TABLE_NAME="${APPLICATION_NAME}"
OUTPUT_FILE="lease-data-$(date +%Y%m%d-%H%M%S).json"

echo "📤 Exporting lease data from DynamoDB table: $TABLE_NAME"
echo "Using AWS Profile: $AWS_PROFILE"
echo "Region: $REGION"
echo ""

# Export all lease items
aws dynamodb scan \
    --table-name "$TABLE_NAME" \
    --region $REGION \
    --profile $AWS_PROFILE \
    --output json > "$OUTPUT_FILE" 2>/dev/null

if [ $? -eq 0 ]; then
    echo "✅ Lease data exported to: $OUTPUT_FILE"
    echo ""
    
    # Show summary
    TOTAL_ITEMS=$(jq '.Items | length' "$OUTPUT_FILE")
    echo "📊 Summary:"
    echo "   Total Leases: $TOTAL_ITEMS"
    echo ""
    
    # Show file size
    FILE_SIZE=$(ls -lh "$OUTPUT_FILE" | awk '{print $5}')
    echo "   File Size: $FILE_SIZE"
    echo ""
    
    # Show breakdown by status
    ACTIVE_ASSIGNED=$(jq '[.Items[] | select(.AssignedTo != null and .AssignedTo.S != "")] | length' "$OUTPUT_FILE")
    SHARD_END_COUNT=$(jq '[.Items[] | select(.Checkpoint.S == "SHARD_END")] | length' "$OUTPUT_FILE")
    UNASSIGNED=$(jq '[.Items[] | select((.AssignedTo == null or .AssignedTo.S == "") and (.Checkpoint.S != "SHARD_END"))] | length' "$OUTPUT_FILE")
    
    echo "   Breakdown:"
    echo "   ├─ Active & Assigned: $ACTIVE_ASSIGNED"
    echo "   ├─ Closed (SHARD_END): $SHARD_END_COUNT"
    echo "   └─ Unassigned (Active): $UNASSIGNED"
else
    echo "❌ Failed to export lease data from table '$TABLE_NAME'"
    echo "   Make sure the table exists and AWS credentials are configured."
    exit 1
fi
