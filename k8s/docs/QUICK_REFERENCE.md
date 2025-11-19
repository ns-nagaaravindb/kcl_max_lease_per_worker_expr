# KDS Lease Manager - Quick Reference

## 🚀 Quick Start

```bash
cd k8s
make setup              # Complete setup
make status             # Check status
make monitor            # Monitor in real-time
```

## 📖 Makefile Commands

### Setup & Deploy

```bash
make setup              # Complete setup (minikube + build + deploy)
make build              # Build Docker image only
make deploy             # Deploy using Helm
make helm-upgrade       # Upgrade existing deployment
make verify             # Check prerequisites
```

### Testing

```bash
make test-shards N=60   # Scale to 60 shards
make test-workers N=5   # Scale to 5 workers
make test-all           # Run all tests
```

### Monitoring

```bash
make monitor            # Real-time monitoring
make logs               # All worker logs
make logs-worker W=0    # Specific worker logs
make status             # Deployment status
make metadata           # Query DynamoDB
make metadata-coordinator  # Coordinator only
```

### Scaling

```bash
make scale-workers N=5  # Scale to 5 workers
make restart-worker W=0 # Restart worker-0
make restart-all        # Restart all workers
```

### Cleanup

```bash
make clean              # Remove all resources
make delete-minikube    # Delete minikube cluster
make restart            # Clean and redeploy
```

## 🔍 Manual kubectl Commands

### Pod Operations

```bash
# List pods
kubectl get pods -n kds-test

# Watch pods
kubectl get pods -n kds-test -w

# Describe pod
kubectl describe pod kds-consumer-0 -n kds-test

# View logs
kubectl logs kds-consumer-0 -n kds-test -f

# Exec into pod
kubectl exec -n kds-test -it kds-consumer-0 -- /bin/sh

# Delete pod (will restart)
kubectl delete pod kds-consumer-0 -n kds-test
```

### StatefulSet Operations

```bash
# Scale StatefulSet
kubectl scale statefulset kds-consumer -n kds-test --replicas=5

# Get StatefulSet info
kubectl get statefulset kds-consumer -n kds-test

# Edit StatefulSet
kubectl edit statefulset kds-consumer -n kds-test
```

### Service Operations

```bash
# List services
kubectl get svc -n kds-test

# Describe service
kubectl describe svc localstack -n kds-test

# Port forward
kubectl port-forward -n kds-test service/localstack 4566:4566
```

## 📊 DynamoDB Queries

### Scan Metadata Table

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal dynamodb scan --table-name kds-consumer-app_meta
```

### Pretty Format

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal dynamodb scan --table-name kds-consumer-app_meta \
  --query 'Items[*].[worker_id.S, max_leases_per_worker.N, shard_count.N, worker_count.N]' \
  --output table
```

### Query Coordinator

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal dynamodb get-item \
  --table-name kds-consumer-app_meta \
  --key '{"worker_id":{"S":"kds-consumer-app_coordinator"}}'
```

### Query Specific Worker

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal dynamodb get-item \
  --table-name kds-consumer-app_meta \
  --key '{"worker_id":{"S":"kds-consumer-0"}}'
```

### List Tables

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal dynamodb list-tables
```

## 🌊 Kinesis Operations

### Describe Stream

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal kinesis describe-stream --stream-name test-stream
```

### List Shards

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

### Update Shard Count

```bash
kubectl exec -n kds-test -it deployment/localstack -- \
  awslocal kinesis update-shard-count \
  --stream-name test-stream \
  --target-shard-count 60 \
  --scaling-type UNIFORM_SCALING
```

## ⚙️ Helm Commands

### Install/Upgrade

```bash
# Install or upgrade
helm upgrade --install kds-lease-manager ./helm/kds-lease-manager \
  --namespace kds-test \
  --create-namespace \
  --wait

# With custom values
helm upgrade --install kds-lease-manager ./helm/kds-lease-manager \
  --namespace kds-test \
  --set consumer.replicaCount=5 \
  --wait
```

### Manage Releases

```bash
# List releases
helm list -n kds-test

# Get values
helm get values kds-lease-manager -n kds-test

# Get manifest
helm get manifest kds-lease-manager -n kds-test

# Uninstall
helm uninstall kds-lease-manager -n kds-test
```

### Template & Lint

```bash
# Generate manifests
helm template kds-lease-manager ./helm/kds-lease-manager \
  --namespace kds-test

# Lint chart
helm lint ./helm/kds-lease-manager

# Dry run
helm upgrade --install kds-lease-manager ./helm/kds-lease-manager \
  --namespace kds-test \
  --dry-run --debug
```

## 🎯 Common Scenarios

### Initial Setup

```bash
make setup
make status
make logs
```

### Test Shard Scaling (30→60)

```bash
make metadata                    # Check before
make test-shards N=60           # Scale shards
make metadata                    # Check after
```

### Test Worker Scaling (3→5)

```bash
make metadata                    # Check before
make test-workers N=5           # Scale workers
make metadata                    # Check after
```

### Debug Worker Issues

```bash
make logs-worker W=0            # View logs
kubectl describe pod kds-consumer-0 -n kds-test  # Check events
make shell-worker W=0           # Open shell
```

### Monitor Real-Time

```bash
# Terminal 1
make monitor

# Terminal 2
make test-shards N=60
```

### Complete Restart

```bash
make clean
make setup
```

## 🔧 Environment Variables

```bash
# Use different namespace
NAMESPACE=my-namespace make deploy

# Use different image
IMAGE_TAG=v2 make build
```

## 📈 Access from Host

### Port Forward LocalStack

```bash
# In one terminal
kubectl port-forward -n kds-test service/localstack 4566:4566

# In another terminal
aws --endpoint-url=http://localhost:4566 kinesis list-streams
aws --endpoint-url=http://localhost:4566 dynamodb list-tables
```

## 🐛 Troubleshooting

### Pods Not Starting

```bash
kubectl get pods -n kds-test
kubectl describe pod <pod-name> -n kds-test
kubectl logs <pod-name> -n kds-test
```

### LocalStack Not Ready

```bash
kubectl logs -n kds-test deployment/localstack
kubectl exec -n kds-test -it deployment/localstack -- \
  curl http://localhost:4566/_localstack/health
```

### Image Issues

```bash
eval $(minikube docker-env)
docker images | grep kds-consumer-test
make build
```

### RBAC Issues

```bash
kubectl get serviceaccount -n kds-test
kubectl get role -n kds-test
kubectl get rolebinding -n kds-test
```

## 💡 Tips

- Use `make help` to see all commands
- Keep `make monitor` running in a separate terminal
- Check both logs and metadata for full picture
- StatefulSet ensures predictable pod naming (0, 1, 2, ...)
- LocalStack data persists only while pod is running

## 📚 Related Files

- `docs/README.md` - Main documentation
- `docs/FLOW_DIAGRAM.md` - Visual diagrams
- `docs/SUMMARY.md` - Complete summary
- `execution_details.md` - Theoretical walkthrough
- `Makefile` - All automation commands

---

**Quick Start: `cd k8s && make setup`** 🚀
