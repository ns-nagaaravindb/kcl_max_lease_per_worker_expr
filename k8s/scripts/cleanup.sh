#!/bin/bash

set -e

NAMESPACE="${NAMESPACE:-kds-test}"

echo "=========================================="
echo "Cleanup KDS Lease Manager Test Environment"
echo "=========================================="
echo "Namespace: $NAMESPACE"
echo ""

echo "Uninstalling Helm release..."
helm uninstall kds-lease-manager -n $NAMESPACE || echo "Release not found"

echo ""
echo "Deleting namespace..."
kubectl delete namespace $NAMESPACE --ignore-not-found=true

echo "✅ All resources deleted"
echo ""

echo "To stop Minikube completely, run:"
echo "  minikube stop"
echo ""
echo "To delete Minikube cluster, run:"
echo "  minikube delete"
echo ""
