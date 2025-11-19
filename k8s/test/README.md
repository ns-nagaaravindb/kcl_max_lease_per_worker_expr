# Test Consumer Application

This directory contains the test application code for the KDS Lease Manager demonstration.

## Contents

```
test-consumer/
├── main.go              # Application entry point
├── lease_manager.go     # Lease manager implementation
├── Dockerfile           # Docker build configuration
└── go.mod              # Go dependencies
```

## Purpose

This is a simplified test consumer application that demonstrates the KDS Lease Manager functionality without the full KCL integration. It's used for:

- **Testing** the lease manager in Minikube
- **Demonstrating** dynamic max leases calculation
- **Validating** the coordinator pattern and race condition handling

## Building

The Docker image is built automatically by the Makefile:

```bash
# From k8s directory
make build
```

Or manually:

```bash
cd test-consumer
eval $(minikube docker-env)
docker build -t kds-consumer-test:latest .
```

## Components

### main.go
- Application entry point
- Health check endpoints (`/health`, `/ready`)
- Worker simulation
- Periodic status logging

### lease_manager.go
- Simplified version of `../kds_lease_manager.go`
- Core lease management logic
- Coordinator pattern implementation
- Dynamic recalculation

### Dockerfile
- Multi-stage build
- Alpine-based runtime
- Minimal image size

### go.mod
- Go dependencies
- AWS SDK v2
- Kubernetes client-go

## Environment Variables

The application expects these environment variables (set by Helm/ConfigMap):

- `AWS_REGION` - AWS region (default: us-east-1)
- `AWS_ACCESS_KEY_ID` - AWS access key
- `AWS_SECRET_ACCESS_KEY` - AWS secret key
- `AWS_ENDPOINT_URL` - LocalStack endpoint
- `STREAM_NAME` - Kinesis stream name
- `APP_NAME` - Application name
- `ENABLE_DYNAMIC_MAX_LEASES` - Enable dynamic lease management
- `POD_NAMESPACE` - Kubernetes namespace
- `POD_NAME` - Pod name (auto-set by K8s)
- `HOSTNAME` - Pod hostname (auto-set by K8s)

## Health Checks

### Liveness Probe
```
GET http://localhost:8080/health
```
Returns 200 OK when application is running

### Readiness Probe
```
GET http://localhost:8080/ready
```
Returns 200 OK when lease manager is initialized

## Deployment

This application is deployed via the Helm chart:

```bash
# From k8s directory
make setup    # Complete setup
make deploy   # Deploy only
```

The Helm chart creates:
- StatefulSet with multiple replicas
- Service for health checks
- ConfigMap with configuration
- RBAC for K8s API access

## Integration with Lease Manager

The application uses the lease manager to:

1. **Query Kinesis** for current shard count
2. **Query K8s API** for current worker count
3. **Calculate** max leases per worker: `min(80, ceil(shards/workers))`
4. **Store** metadata in DynamoDB
5. **Coordinate** with other workers using conditional writes

## For Development

### Local Testing

```bash
# Set up environment
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_ENDPOINT_URL=http://localhost:4566
export STREAM_NAME=test-stream
export APP_NAME=kds-consumer-app
export ENABLE_DYNAMIC_MAX_LEASES=true

# Run
go run *.go
```

### Debugging

```bash
# Shell into running pod
make shell-worker W=0

# View logs
make logs-worker W=0

# Check environment
kubectl exec -n kds-test kds-consumer-0 -- env
```

## Related Files

- **Helm Chart**: `../helm/kds-lease-manager/`
- **Production Code**: `../kds_lease_manager.go`
- **Documentation**: `../docs/README.md`
- **Scripts**: `../scripts/`

---

**Note**: This is a test/demo application. For production use, integrate `kds_lease_manager.go` with your actual KCL consumer.
