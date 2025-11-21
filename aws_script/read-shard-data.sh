#!/bin/bash

# # Basic usage (prints to stdout)
# ./read-shard-data.sh <stream_name> <shard_id>

# # Read from beginning of shard
# ITERATOR_TYPE=TRIM_HORIZON ./read-shard-data.sh <stream_name> <shard_id>

# # Save to file
# OUTPUT_FILE=shard-data.json ./read-shard-data.sh <stream_name> <shard_id>

# # Custom AWS profile/region
# AWS_PROFILE=myprofile AWS_REGION=us-east-1 ./read-shard-data.sh <stream_name> <shard_id>

# Check if stream name and shard ID are provided
if [ -z "$1" ] || [ -z "$2" ]; then
    echo "❌ Error: Stream name and shard ID are required"
    echo ""
    echo "Usage: $0 <stream_name> <shard_id>"
    echo ""
    echo "Example: $0 my-kinesis-stream shardId-000000000000"
    echo ""
    echo "Environment variables (optional):"
    echo "  AWS_PROFILE  - AWS profile to use (default: prod)"
    echo "  AWS_REGION   - AWS region (default: eu-west-2)"
    echo "  ITERATOR_TYPE - Shard iterator type (default: LATEST)"
    echo "                  Options: TRIM_HORIZON, LATEST, AT_SEQUENCE_NUMBER, AFTER_SEQUENCE_NUMBER, AT_TIMESTAMP"
    echo "  MAX_RECORDS  - Maximum number of records to read per request (default: 10000)"
    echo "  OUTPUT_FILE  - Output file to save records (optional, default: print to stdout)"
    exit 1
fi

STREAM_NAME="$1"
SHARD_ID="$2"
AWS_PROFILE="${AWS_PROFILE:-prod}"
REGION="${AWS_REGION:-eu-west-2}"
ITERATOR_TYPE="${ITERATOR_TYPE:-LATEST}"
MAX_RECORDS="${MAX_RECORDS:-10000}"
OUTPUT_FILE="${OUTPUT_FILE:-}"

echo "📖 Reading data from Kinesis Data Stream shard"
echo "   Stream: $STREAM_NAME"
echo "   Shard ID: $SHARD_ID"
echo "   Iterator Type: $ITERATOR_TYPE"
echo "   Using AWS Profile: $AWS_PROFILE"
echo "   Region: $REGION"
echo ""

# Get shard iterator
echo "🔍 Getting shard iterator..."
SHARD_ITERATOR=$(aws kinesis get-shard-iterator \
    --stream-name "$STREAM_NAME" \
    --shard-id "$SHARD_ID" \
    --shard-iterator-type "$ITERATOR_TYPE" \
    --region $REGION \
    --profile $AWS_PROFILE \
    --query 'ShardIterator' \
    --output text 2>/dev/null)

if [ $? -ne 0 ] || [ -z "$SHARD_ITERATOR" ] || [ "$SHARD_ITERATOR" == "None" ]; then
    echo "❌ Failed to get shard iterator for shard '$SHARD_ID'"
    echo "   Make sure:"
    echo "   - The stream '$STREAM_NAME' exists"
    echo "   - The shard ID '$SHARD_ID' is valid"
    echo "   - AWS credentials are configured"
    echo "   - The shard is not closed (if using TRIM_HORIZON)"
    exit 1
fi

echo "✅ Shard iterator obtained"
echo ""

# Initialize counters
TOTAL_RECORDS=0
BATCH_COUNT=0
RECORDS_JSON=""

# Read records in batches
echo "📥 Reading records..."
while [ -n "$SHARD_ITERATOR" ] && [ "$SHARD_ITERATOR" != "None" ]; do
    BATCH_COUNT=$((BATCH_COUNT + 1))
    
    # Get records
    RECORDS_RESPONSE=$(aws kinesis get-records \
        --shard-iterator "$SHARD_ITERATOR" \
        --limit $MAX_RECORDS \
        --region $REGION \
        --profile $AWS_PROFILE \
        --output json 2>/dev/null)
    
    if [ $? -ne 0 ]; then
        echo "❌ Failed to get records from shard iterator"
        break
    fi
    
    # Extract records and next iterator
    RECORDS=$(echo "$RECORDS_RESPONSE" | jq -r '.Records // []')
    SHARD_ITERATOR=$(echo "$RECORDS_RESPONSE" | jq -r '.NextShardIterator // empty')
    MILLIS_BEHIND_LATEST=$(echo "$RECORDS_RESPONSE" | jq -r '.MillisBehindLatest // 0')
    
    RECORD_COUNT=$(echo "$RECORDS" | jq -r 'length')
    
    if [ "$RECORD_COUNT" -gt 0 ]; then
        TOTAL_RECORDS=$((TOTAL_RECORDS + RECORD_COUNT))
        echo "   Batch $BATCH_COUNT: Read $RECORD_COUNT record(s) (Total: $TOTAL_RECORDS)"
        
        # Accumulate records
        if [ -z "$RECORDS_JSON" ]; then
            RECORDS_JSON="$RECORDS"
        else
            RECORDS_JSON=$(echo "$RECORDS_JSON" | jq ". + $RECORDS")
        fi
    else
        echo "   Batch $BATCH_COUNT: No records found"
    fi
    
    # Show progress if behind latest
    if [ "$MILLIS_BEHIND_LATEST" -gt 0 ]; then
        SECONDS_BEHIND=$(awk "BEGIN {printf \"%.2f\", $MILLIS_BEHIND_LATEST / 1000}")
        echo "   ⏱️  MillisBehindLatest: $MILLIS_BEHIND_LATEST ms ($SECONDS_BEHIND seconds)"
    fi
    
    # Break if no more records or no next iterator
    if [ "$RECORD_COUNT" -eq 0 ] || [ -z "$SHARD_ITERATOR" ] || [ "$SHARD_ITERATOR" == "None" ]; then
        break
    fi
    
    # Small delay to avoid throttling
    sleep 0.1
done

echo ""
echo "✅ Finished reading records"
echo ""

# Display summary
echo "📊 Summary:"
echo "   Total Records Read: $TOTAL_RECORDS"
echo "   Batches Processed: $BATCH_COUNT"
echo ""

if [ "$TOTAL_RECORDS" -eq 0 ]; then
    echo "   ℹ️  No records found in this shard"
    echo "   💡 Tip: Try using TRIM_HORIZON iterator type to read from the beginning"
    exit 0
fi

# Output records
if [ -n "$OUTPUT_FILE" ]; then
    # Save to file
    echo "$RECORDS_JSON" | jq '.' > "$OUTPUT_FILE"
    FILE_SIZE=$(ls -lh "$OUTPUT_FILE" | awk '{print $5}')
    echo "💾 Records saved to: $OUTPUT_FILE"
    echo "   File Size: $FILE_SIZE"
    echo ""
    echo "   To view records:"
    echo "   cat $OUTPUT_FILE | jq '.'"
else
    # Print to stdout
    echo "📋 Records:"
    echo ""
    echo "$RECORDS_JSON" | jq -r '.[] | 
        "   Record #\(.SequenceNumber // "N/A"):" +
        "\n      Partition Key: \(.PartitionKey // "N/A")" +
        "\n      Approximate Arrival Timestamp: \(.ApproximateArrivalTimestamp // "N/A")" +
        "\n      Data: \(.Data // "" | @base64d | .[0:100] // "")" +
        "\n"'
    
    # Show data preview
    echo ""
    echo "💡 Tip: Set OUTPUT_FILE environment variable to save records to a file"
    echo "   Example: OUTPUT_FILE=shard-data.json $0 $STREAM_NAME $SHARD_ID"
fi

echo ""
echo "=========================================="

