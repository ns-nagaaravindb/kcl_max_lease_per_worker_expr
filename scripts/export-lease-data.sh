#!/bin/bash

APPLICATION_NAME="mohan-kcl-consumer"
TABLE_NAME="${APPLICATION_NAME}"
CONTAINER_NAME="localstack-mohan-experiment"
REGION="us-east-1"
OUTPUT_FILE="lease-data-$(date +%Y%m%d-%H%M%S).json"

echo "📤 Exporting lease data from DynamoDB table: $TABLE_NAME"
echo ""

# Export all lease items
docker exec $CONTAINER_NAME awslocal dynamodb scan \
    --table-name "$TABLE_NAME" \
    --region $REGION \
    --output json > "$OUTPUT_FILE"

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
else
    echo "❌ Failed to export lease data"
    exit 1
fi

