#!/bin/bash

set -e

NAMESPACE="${NAMESPACE:-kds-test}"

echo "=========================================="
echo "KDS Lease Manager Test Environment Setup"
echo "=========================================="
echo "Namespace: $NAMESPACE"
echo ""

# Check prerequisites
echo "Checking prerequisites..."

if ! command -v minikube &> /dev/null; then
    echo "❌ minikube is not installed. Please install it first."
    exit 1
fi

if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed. Please install it first."
    exit 1
fi

if ! command -v docker &> /dev/null; then
    echo "❌ docker is not installed. Please install it first."
    exit 1
fi

if ! command -v helm &> /dev/null; then
    echo "❌ helm is not installed. Please install it first."
    exit 1
fi

echo "✅ All prerequisites found"
echo ""

# Start minikube if not running
echo "Starting Minikube..."
if minikube status | grep -q "Running"; then
    echo "✅ Minikube is already running"
else
    minikube start --memory=4096 --cpus=2
    echo "✅ Minikube started"
fi
echo ""

# Configure Docker to use Minikube's Docker daemon
echo "Configuring Docker environment for Minikube..."
eval $(minikube docker-env)
echo "✅ Docker configured to use Minikube's daemon"
echo ""

# Build the test consumer image
echo "Building test consumer Docker image..."
cd test/test-consumer
docker build -t kds-consumer-test:latest .
echo "✅ Docker image built: kds-consumer-test:latest"
cd ../../
echo ""

# Deploy using Helm
echo "Deploying KDS Lease Manager using Helm..."
helm upgrade --install kds-lease-manager helm/kds-lease-manager \
  --namespace $NAMESPACE \
  --create-namespace \
  --wait \
  --timeout 5m

echo "✅ Helm deployment complete"
echo ""

echo "=========================================="
echo "Setup Complete!"
echo "=========================================="
echo ""
echo "Useful commands:"
echo ""
echo "  # Watch pod status"
echo "  kubectl get pods -n $NAMESPACE -w"
echo ""
echo "  # View logs from all workers"
echo "  kubectl logs -n $NAMESPACE -l app=kds-consumer --all-containers=true -f"
echo ""
echo "  # View logs from specific worker"
echo "  kubectl logs -n $NAMESPACE kds-consumer-0 -f"
echo ""
echo "  # Check DynamoDB metadata table"
echo "  kubectl exec -n $NAMESPACE -it deployment/localstack -- awslocal dynamodb scan --table-name kds-consumer-app_meta"
echo ""
echo "  # Scale workers"
echo "  kubectl scale statefulset kds-consumer -n $NAMESPACE --replicas=5"
echo ""
echo "  # Clean up everything"
echo "  make clean"
echo ""
