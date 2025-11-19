# KDS Lease Manager - Kubernetes Test Environment

Complete Kubernetes/Minikube test setup for the KDS Lease Manager with LocalStack using Helm deployment.

## 📋 Overview

This test environment demonstrates the KDS Lease Manager behavior described in `../execution_details.md`:
- **LocalStack** for AWS Kinesis Data Streams and DynamoDB
- **Helm** for simplified deployment and configuration management
- **StatefulSet** with 3 consumer workers (scalable via Helm values)
- **Automatic lease management** with dynamic `max_leases_per_worker` calculation
- **Makefile** for easy command execution
- **Namespace isolation** using `kds-test` namespace

## 🚀 Quick Start

### Prerequisites

- [Minikube](https://minikube.sigs.k8s.io/docs/start/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Docker](https://docs.docker.com/get-docker/)
- [Helm](https://helm.sh/docs/intro/install/) v3+

### One-Command Setup

```bash
# From the k8s directory
make setup
```

This will:
1. ✅ Start Minikube (if not running)
2. ✅ Build Docker image
3. ✅ Deploy everything using Helm
4. ✅ Create Kinesis stream with 30 shards
5. ✅ Deploy 3 consumer workers in `kds-test` namespace

### Verify Deployment

```bash
# Check status
make status

# View logs
make logs

# Monitor metadata in real-time
make monitor
```

## 📁 Directory Structure

```
k8s/
├── Makefile                    # Main automation (start here!)
├── execution_details.md        # Theoretical walkthrough
├── kds_lease_manager.go        # Production implementation
│
├── docs/                       # Documentation
│   ├── README.md              # This file
│   ├── QUICK_REFERENCE.md     # Command cheat sheet
│   ├── FLOW_DIAGRAM.md        # Visual flow diagrams
│   └── SUMMARY.md             # Complete summary
│
├── scripts/                    # Shell scripts
│   ├── setup.sh               # Complete setup
│   ├── cleanup.sh             # Cleanup resources
│   ├── test-scale-shards.sh   # Test shard scaling
│   ├── test-scale-workers.sh  # Test worker scaling
│   └── monitor.sh             # Real-time monitoring
│
├── helm/                       # Helm chart
│   └── kds-lease-manager/
│       ├── Chart.yaml
│       ├── values.yaml        # Configuration values
│       └── templates/         # Kubernetes manifests
│           ├── namespace.yaml
│           ├── rbac.yaml
│           ├── configmap.yaml
│           ├── localstack-deployment.yaml
│           ├── statefulset.yaml
│           └── init-job.yaml
│
└── test/test-consumer/         # Test application
    ├── main.go
    ├── lease_manager.go
    ├── Dockerfile
    └── go.mod
```

## 🎯 Makefile Commands

### Setup Commands

```bash
make setup              # Complete setup (minikube + build + deploy)
make build              # Build Docker image only
make deploy             # Deploy using Helm (builds image first)
make install            # Alias for deploy
```

### Test Commands

```bash
make test-shards N=60   # Test shard scaling to 60 shards
make test-workers N=5   # Test worker scaling to 5 workers
make test-all           # Run all tests
```

### Monitoring Commands

```bash
make monitor            # Real-time metadata monitoring
make logs               # View logs from all workers
make logs-worker W=0    # View logs from specific worker
make status             # Show deployment status
make metadata           # Query DynamoDB metadata table
```

### Scaling Commands

```bash
make scale-workers N=5  # Scale to 5 workers
make restart-worker W=0 # Restart specific worker
make restart-all        # Restart all workers
```

### Cleanup Commands

```bash
make clean              # Remove all resources (keep minikube)
make delete-minikube    # Delete minikube cluster
make restart            # Clean and redeploy
```

### Helm Commands

```bash
make helm-lint          # Lint Helm chart
make helm-template      # Generate Kubernetes manifests
make helm-upgrade       # Upgrade existing deployment
```

### Utility Commands

```bash
make verify             # Verify prerequisites
make start-minikube     # Start minikube
make stop-minikube      # Stop minikube
make port-forward-localstack  # Access LocalStack from host
make shell-localstack   # Open shell in LocalStack pod
make shell-worker W=0   # Open shell in worker pod
make docs               # Show documentation locations
make help               # Show all commands
```

## 🧪 Testing Scenarios

### Scenario 1: Initial Deployment (Phase 1)

```bash
# Setup with default configuration
make setup

# Expected: 30 shards, 3 workers → max_leases_per_worker = 10
make metadata
```

### Scenario 2: Shard Scaling (Phase 2 from execution_details.md)

```bash
# Scale shards from 30 to 60
make test-shards N=60

# Expected: 60 shards, 3 workers → max_leases_per_worker = 20
make metadata
```

### Scenario 3: Worker Scaling

```bash
# Scale workers from 3 to 5
make test-workers N=5

# Expected: 30 shards, 5 workers → max_leases_per_worker = 6
make metadata
```

### Scenario 4: Test 80 Lease Limit

```bash
# Scale to 300 shards with 3 workers
make test-shards N=300

# Expected: ceil(300/3) = 100, but capped at 80
make logs-worker W=0 | grep "max leases"
```

### Scenario 5: Combined Scaling

```bash
# Scale both shards and workers
make test-shards N=100
make test-workers N=5

# Expected: ceil(100/5) = 20
make metadata
```

## 📊 Real-Time Monitoring

```bash
# Start the monitor (refreshes every 10 seconds)
make monitor
```

Output shows:
- All worker metadata
- Coordinator metadata (detailed)
- Pod status

Press `Ctrl+C` to exit.

## 🔧 Configuration

### Helm Values

Edit `helm/kds-lease-manager/values.yaml` to configure:

```yaml
namespace: kds-test

consumer:
  replicaCount: 3              # Number of workers
  stream:
    name: test-stream
    initialShardCount: 30      # Initial shard count
  app:
    name: kds-consumer-app
    enableDynamicMaxLeases: true
```

### Deploy with Custom Values

```bash
# Deploy with 5 workers
helm upgrade --install kds-lease-manager ./helm/kds-lease-manager \
  --namespace kds-test \
  --create-namespace \
  --set consumer.replicaCount=5 \
  --wait

# Or use custom values file
helm upgrade --install kds-lease-manager ./helm/kds-lease-manager \
  --namespace kds-test \
  --create-namespace \
  --values my-values.yaml \
  --wait
```

## 🔍 Manual Queries

### Check Metadata Table

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal dynamodb scan --table-name kds-consumer-app_meta
```

### Check Coordinator

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal dynamodb get-item \
  --table-name kds-consumer-app_meta \
  --key '{"worker_id":{"S":"kds-consumer-app_coordinator"}}'
```

### Check Stream Shards

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal kinesis list-shards --stream-name test-stream
```

### Count Shards

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal kinesis list-shards --stream-name test-stream \
  --query 'length(Shards)' --output text
```

## 📝 Formula

The lease manager uses this formula:

```
max_leases_per_worker = min(80, ceil(shard_count / worker_count))
```

Examples:
- 30 shards, 3 workers: `ceil(30/3) = 10`
- 60 shards, 3 workers: `ceil(60/3) = 20`
- 30 shards, 5 workers: `ceil(30/5) = 6`
- 300 shards, 3 workers: `min(80, ceil(300/3)) = min(80, 100) = 80`

## 🔄 Typical Workflow

```bash
# 1. Initial setup
make setup

# 2. Verify deployment
make status

# 3. Monitor in real-time (in separate terminal)
make monitor

# 4. Run tests
make test-shards N=60
make test-workers N=5

# 5. View logs
make logs

# 6. Cleanup
make clean
```

## 🐛 Troubleshooting

### Pods Not Starting

```bash
# Check pod status
make status

# Check specific pod
kubectl describe pod kds-consumer-0 -n kds-test

# Check logs
make logs-worker W=0
```

### LocalStack Issues

```bash
# Check LocalStack logs
kubectl logs -n kds-test deployment/localstack

# Verify LocalStack health
kubectl exec -n kds-test -it deployment/localstack -- \
  curl http://localhost:4566/_localstack/health

# Restart LocalStack
kubectl rollout restart deployment/localstack -n kds-test
```

### Image Issues

```bash
# Rebuild image
make build

# Verify image
eval $(minikube docker-env)
docker images | grep kds-consumer-test
```

### Helm Issues

```bash
# Lint chart
make helm-lint

# View generated manifests
make helm-template

# Check Helm releases
helm list -n kds-test
```

## 🎓 Key Concepts

### 1. Coordinator Pattern
- One worker becomes coordinator using DynamoDB conditional writes
- Others read coordinator's computed value
- Race-condition safe

### 2. Dynamic Recalculation
- Workers detect shard/worker count changes
- Automatic recalculation and updates
- No manual intervention needed

### 3. Kubernetes Integration
- RBAC for K8s API access
- Queries StatefulSet replica count
- Stable pod identities

### 4. Namespace Isolation
- All resources in `kds-test` namespace
- Easy cleanup without affecting other workloads
- Clear resource boundaries

## 📚 Documentation

- **[QUICK_REFERENCE.md](QUICK_REFERENCE.md)** - Command cheat sheet
- **[FLOW_DIAGRAM.md](FLOW_DIAGRAM.md)** - Visual flow diagrams
- **[SUMMARY.md](SUMMARY.md)** - Complete summary
- **[../execution_details.md](../execution_details.md)** - Theoretical walkthrough
- **[../kds_lease_manager.go](../kds_lease_manager.go)** - Production code

## 🚧 Common Tasks

### Change Namespace

```bash
# Use different namespace
NAMESPACE=my-namespace make deploy

# Or update values.yaml
namespace: my-namespace
```

### Scale Workers

```bash
# Using kubectl
make scale-workers N=5

# Or update Helm values
helm upgrade kds-lease-manager ./helm/kds-lease-manager \
  -n kds-test \
  --set consumer.replicaCount=5 \
  --wait
```

### Access LocalStack from Host

```bash
# Port forward in background
make port-forward-localstack &

# Use AWS CLI from host
aws --endpoint-url=http://localhost:4566 kinesis list-streams
```

### Debug Worker

```bash
# Open shell in worker
make shell-worker W=0

# Check environment variables
kubectl exec -n kds-test kds-consumer-0 -- env | grep AWS

# Check health endpoints
kubectl exec -n kds-test kds-consumer-0 -- wget -O- http://localhost:8080/health
```

## 🎉 Next Steps

1. Run through all test scenarios
2. Modify Helm values for different configurations
3. Integrate with your actual consumer application
4. Adapt for production use (replace LocalStack with real AWS)
5. Add monitoring/alerting

---

**For detailed command reference, see [QUICK_REFERENCE.md](QUICK_REFERENCE.md)**

**For visual diagrams, see [FLOW_DIAGRAM.md](FLOW_DIAGRAM.md)**

**To get started: `make setup`** 🚀
