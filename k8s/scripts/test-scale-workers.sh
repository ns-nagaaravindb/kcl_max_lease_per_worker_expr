#!/bin/bash

# Script to test worker scaling scenario

set -e

NAMESPACE="${NAMESPACE:-kds-test}"

if [ -z "$1" ]; then
    echo "Usage: $0 <new_worker_count>"
    echo "Example: $0 5"
    exit 1
fi

NEW_WORKER_COUNT=$1

echo "=========================================="
echo "Testing Worker Scaling Scenario"
echo "=========================================="
echo "Namespace: $NAMESPACE"
echo ""
echo "This script will:"
echo "1. Scale the StatefulSet to $NEW_WORKER_COUNT replicas"
echo "2. Wait for new pods to be ready"
echo "3. Monitor the metadata updates"
echo ""

echo "Step 1: Current metadata state BEFORE scaling..."
kubectl exec -n $NAMESPACE deployment/localstack -- awslocal dynamodb scan \
    --table-name kds-consumer-app_meta \
    --output json | jq -r '["WORKER_ID", "MAX_LEASES", "SHARD_COUNT", "WORKER_COUNT", "LAST_UPDATED"], ["---------", "----------", "-----------", "------------", "------------"], (.Items[] | [.worker_id.S, .max_leases_per_worker.N, .shard_count.N, .worker_count.N, .last_update_time.S // "N/A"]) | @tsv' | column -t

echo ""
echo "Step 2: Scaling StatefulSet to $NEW_WORKER_COUNT replicas..."
kubectl scale statefulset kds-consumer -n $NAMESPACE --replicas=$NEW_WORKER_COUNT

echo ""
echo "Step 3: Waiting for all pods to be ready..."
kubectl wait --for=condition=ready pod -l app=kds-consumer -n $NAMESPACE --timeout=180s --all

echo ""
echo "Step 4: Performing rolling restart of all workers to detect the change..."
kubectl rollout restart statefulset/kds-consumer -n $NAMESPACE

echo "Waiting for rolling restart to complete..."
kubectl rollout status statefulset/kds-consumer -n $NAMESPACE --timeout=180s

echo ""
echo "Giving workers time to initialize and detect changes..."
sleep 15

echo ""
echo "Step 5: Checking metadata state AFTER scaling..."
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
echo "  New worker count: $NEW_WORKER_COUNT"
echo "  Check max_leases_per_worker in the table above"
echo "  Expected: min(80, ceil(shard_count / $NEW_WORKER_COUNT))"
echo ""
