#!/bin/bash

# Script to scale the KDS stream and test automatic recalculation

set -e

NAMESPACE="${NAMESPACE:-kds-test}"

if [ -z "$1" ]; then
    echo "Usage: $0 <new_shard_count>"
    echo "Example: $0 60"
    exit 1
fi

NEW_SHARD_COUNT=$1

echo "=========================================="
echo "Testing Shard Scaling Scenario"
echo "=========================================="
echo "Namespace: $NAMESPACE"
echo ""
echo "This script will:"
echo "1. Update the Kinesis stream to $NEW_SHARD_COUNT shards"
echo "2. Restart one worker pod to detect the change"
echo "3. Monitor the metadata updates"
echo ""

# Update shard count in LocalStack
echo "Step 1: Updating Kinesis stream to $NEW_SHARD_COUNT shards..."
kubectl exec -n $NAMESPACE -it deployment/localstack -- sh -c "
    export AWS_ACCESS_KEY_ID=test
    export AWS_SECRET_ACCESS_KEY=test
    export AWS_DEFAULT_REGION=us-east-1
    
    echo 'Updating stream shard count to $NEW_SHARD_COUNT...'
    aws kinesis update-shard-count \
        --stream-name test-stream \
        --target-shard-count $NEW_SHARD_COUNT \
        --scaling-type UNIFORM_SCALING \
        --endpoint-url http://localhost:4566
    
    echo 'Stream update initiated. Waiting for it to complete...'
    sleep 5
"

echo ""
echo "Step 2: Checking current metadata state BEFORE restart..."
kubectl exec -n $NAMESPACE deployment/localstack -- awslocal dynamodb scan \
    --table-name kds-consumer-app_meta \
    --output json | jq -r '["WORKER_ID", "MAX_LEASES", "SHARD_COUNT", "WORKER_COUNT", "LAST_UPDATED"], ["---------", "----------", "-----------", "------------", "------------"], (.Items[] | [.worker_id.S, .max_leases_per_worker.N, .shard_count.N, .worker_count.N, .last_update_time.S // "N/A"]) | @tsv' | column -t

echo ""
echo "Step 3: Performing rolling restart of all workers to detect the change..."
kubectl rollout restart statefulset/kds-consumer -n $NAMESPACE

echo "Waiting for rolling restart to complete..."
kubectl rollout status statefulset/kds-consumer -n $NAMESPACE --timeout=180s

echo ""
echo "Step 4: Checking metadata state AFTER restart (may take a few seconds)..."
sleep 10

echo ""
echo "Final Metadata State:"
kubectl exec -n $NAMESPACE deployment/localstack -- awslocal dynamodb scan \
    --table-name kds-consumer-app_meta \
    --output json | jq -r '["WORKER_ID", "MAX_LEASES", "SHARD_COUNT", "WORKER_COUNT", "LAST_UPDATED"], ["---------", "----------", "-----------", "------------", "------------"], (.Items[] | [.worker_id.S, .max_leases_per_worker.N, .shard_count.N, .worker_count.N, .last_update_time.S // "N/A"]) | @tsv' | column -t

echo ""
echo "=========================================="
echo "Test Complete!"
echo "=========================================="
echo ""
echo "Summary:"
echo "  New shard count: $NEW_SHARD_COUNT"
echo "  Check max_leases_per_worker in the table above"
echo "  Expected: min(80, ceil($NEW_SHARD_COUNT / worker_count))"
echo ""
