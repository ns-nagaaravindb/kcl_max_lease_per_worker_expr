# KDS Lease Manager Test Environment

Complete Kubernetes/Minikube test setup for the KDS Lease Manager with LocalStack.

## Structure

```
k8s/test/
├── README.md                    # Complete documentation
├── QUICK_REFERENCE.md           # Quick command reference
│
├── Kubernetes Manifests:
├── localstack-deployment.yaml   # LocalStack (Kinesis + DynamoDB)
├── statefulset.yaml            # Consumer workers (3 replicas)
├── rbac.yaml                   # RBAC for K8s API access
├── configmap.yaml              # Configuration
├── init-job.yaml               # Initialize stream and tables
│
├── Scripts:
├── setup.sh                    # Complete setup (run this first)
├── cleanup.sh                  # Remove all resources
├── test-scale-shards.sh        # Test shard scaling
├── test-scale-workers.sh       # Test worker scaling
├── monitor.sh                  # Real-time monitoring
│
└── test-consumer/              # Test application
    ├── main.go                 # Application code
    ├── lease_manager.go        # Lease manager implementation
    ├── Dockerfile              # Docker build
    └── go.mod                  # Dependencies
```

## Quick Start

```bash
cd k8s/test

# Setup everything
./setup.sh

# Monitor in real-time
./monitor.sh

# Test scenarios
./test-scale-shards.sh 60
./test-scale-workers.sh 5

# Cleanup
./cleanup.sh
```

## What This Tests

This environment demonstrates the exact behavior described in `../execution_details.md`:

1. **Initial Deployment** (Phase 1)
   - 30 shards, 3 workers
   - Formula: `ceil(30/3) = 10` max leases per worker

2. **Shard Scaling** (Phase 2)
   - Scale to 60 shards
   - Formula: `ceil(60/3) = 20` max leases per worker
   - Workers automatically detect and recalculate

3. **Worker Scaling**
   - Scale to 5 workers
   - Formula: `ceil(30/5) = 6` max leases per worker

4. **Hard Limit**
   - 300 shards, 3 workers
   - Formula: `min(80, ceil(300/3)) = min(80, 100) = 80`

See **README.md** for detailed documentation and **QUICK_REFERENCE.md** for command cheat sheet.

