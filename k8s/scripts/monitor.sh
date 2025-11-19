#!/bin/bash

# Script to monitor the DynamoDB metadata table in real-time

NAMESPACE="${NAMESPACE:-kds-test}"

echo "=========================================="
echo "KDS Lease Manager - Metadata Monitor"
echo "=========================================="
echo "Namespace: $NAMESPACE"
echo ""

while true; do
    clear
    echo "=========================================="
    echo "Metadata Table State - $(date)"
    echo "=========================================="
    echo "Namespace: $NAMESPACE"
    echo ""
    
    echo "All Worker Metadata:"
    echo "-------------------------------------------"
    kubectl exec -n $NAMESPACE deployment/localstack -- awslocal dynamodb scan \
        --table-name kds-consumer-app_meta \
        --query 'Items[*].[worker_id.S, max_leases_per_worker.N, shard_count.N, worker_count.N]' \
        --output table 2>/dev/null || echo "Error reading metadata"
    
    echo ""
    echo "Coordinator Metadata (Detailed):"
    echo "-------------------------------------------"
    kubectl exec -n $NAMESPACE deployment/localstack -- awslocal dynamodb get-item \
        --table-name kds-consumer-app_meta \
        --key '{"worker_id":{"S":"kds-consumer-app_coordinator"}}' 2>/dev/null || echo "No coordinator metadata found"
    
    echo ""
    echo "Pod Status:"
    echo "-------------------------------------------"
    kubectl get pods -n $NAMESPACE -l app=kds-consumer
    
    echo ""
    echo "Press Ctrl+C to exit. Refreshing in 10 seconds..."
    sleep 10
done
