# KDS Lease Manager - Kubernetes Test Environment Summary

## ✅ Created Files

### Documentation (3 files)
- **INDEX.md** - Quick overview and structure
- **README.md** - Complete documentation with detailed instructions  
- **QUICK_REFERENCE.md** - Command cheat sheet

### Kubernetes Manifests (5 files)
- **localstack-deployment.yaml** - LocalStack deployment with Kinesis + DynamoDB
- **statefulset.yaml** - Consumer workers StatefulSet (3 replicas)
- **rbac.yaml** - ServiceAccount, Role, RoleBinding for K8s API access
- **configmap.yaml** - Configuration (stream name, region, AWS credentials)
- **init-job.yaml** - Initialization job (creates stream with 30 shards)

### Scripts (5 files)
- **setup.sh** - Complete automated setup
- **cleanup.sh** - Remove all resources
- **test-scale-shards.sh** - Test shard scaling (e.g., 30→60 shards)
- **test-scale-workers.sh** - Test worker scaling (e.g., 3→5 workers)
- **monitor.sh** - Real-time monitoring dashboard

### Application (4 files in test-consumer/)
- **main.go** - Application entry point with health checks (306 lines)
- **lease_manager.go** - Lease manager implementation (481 lines)
- **Dockerfile** - Multi-stage Docker build
- **go.mod** - Go dependencies

## 📊 Total: 17 files created

## 🎯 What You Can Do Now

### 1. Setup and Run

```bash
cd k8s/test
./setup.sh
```

This will:
- ✅ Start Minikube
- ✅ Build Docker image
- ✅ Deploy LocalStack
- ✅ Create Kinesis stream (30 shards)
- ✅ Deploy 3 consumer workers
- ✅ Initialize DynamoDB tables

### 2. Test Scenarios from execution_details.md

#### Phase 1: Initial Deployment (30 shards, 3 workers)
```bash
kubectl get pods
kubectl logs -l app=kds-consumer -f
```
**Expected**: `max_leases_per_worker = 10` (ceil(30/3))

#### Phase 2: Shard Count Increases to 60
```bash
./test-scale-shards.sh 60
```
**Expected**: `max_leases_per_worker = 20` (ceil(60/3))

#### Additional: Worker Scaling
```bash
./test-scale-workers.sh 5
```
**Expected**: `max_leases_per_worker = 6` (ceil(30/5))

#### Test 80 Lease Limit
```bash
./test-scale-shards.sh 300
```
**Expected**: `max_leases_per_worker = 80` (min(80, ceil(300/3)))

### 3. Monitor in Real-Time

```bash
./monitor.sh
```

Shows:
- Current metadata table state
- Coordinator metadata
- Pod status
- Auto-refreshes every 10 seconds

### 4. View Metadata

```bash
kubectl exec -it deployment/localstack -- \
  awslocal dynamodb scan --table-name kds-consumer-app_meta
```

### 5. Cleanup

```bash
./cleanup.sh
```

## 🧪 Test Coverage

This environment tests all scenarios from `execution_details.md`:

| Scenario | Test Command | Expected Result |
|----------|-------------|-----------------|
| Phase 1: Initial (30/3) | `setup.sh` | max_leases = 10 |
| Phase 2: Scale shards (60/3) | `test-scale-shards.sh 60` | max_leases = 20 |
| Scale workers (30/5) | `test-scale-workers.sh 5` | max_leases = 6 |
| 80 limit (300/3) | `test-scale-shards.sh 300` | max_leases = 80 |
| Multiple restarts | Delete all pods | Race-safe updates |
| Config change detection | Scale & restart | Auto-recalculation |

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Minikube Cluster (localhost)                │
│                                                          │
│  ┌──────────────┐                                       │
│  │  LocalStack  │  (Kinesis + DynamoDB)                 │
│  │  :4566       │                                       │
│  └──────┬───────┘                                       │
│         │                                                │
│         │ AWS API Calls                                 │
│         │                                                │
│  ┌──────┴────────┬──────────────┬──────────────┐      │
│  │               │              │              │       │
│  │ consumer-0    │ consumer-1   │ consumer-2   │       │
│  │ (Pod)         │ (Pod)        │ (Pod)        │       │
│  │               │              │              │       │
│  │ • Reads shard count from Kinesis                    │
│  │ • Reads worker count from K8s API                   │
│  │ • Calculates max_leases_per_worker                  │
│  │ • Stores in DynamoDB metadata table                 │
│  │                                                      │
│  └──────────────────────────────────────────────────────┘
```

## 🔑 Key Features Demonstrated

1. **Coordinator Pattern**
   - One worker becomes coordinator using DynamoDB conditional writes
   - Other workers read coordinator's computed value
   - Race-condition safe

2. **Dynamic Recalculation**
   - Workers detect shard count changes
   - Workers detect worker count changes via K8s API
   - Automatic recalculation and metadata updates

3. **Formula Application**
   ```
   max_leases_per_worker = min(80, ceil(shard_count / worker_count))
   ```

4. **Kubernetes Integration**
   - RBAC for K8s API access
   - Queries StatefulSet replica count
   - Stable pod identities

5. **Health Checks**
   - Liveness probe on /health
   - Readiness probe on /ready

## 📚 Documentation Structure

- **INDEX.md** - Start here for quick overview
- **README.md** - Complete guide with all details
- **QUICK_REFERENCE.md** - Command cheat sheet
- **../execution_details.md** - Theoretical walkthrough
- **../kds_lease_manager.go** - Production implementation

## 🎓 Learning Path

1. Read `../execution_details.md` to understand the theory
2. Review `README.md` to understand the test setup
3. Run `./setup.sh` to deploy the environment
4. Follow test scenarios to see it in action
5. Use `monitor.sh` to observe state changes in real-time
6. Review `test-consumer/lease_manager.go` to see the implementation

## 🔧 Prerequisites

- Minikube installed
- kubectl installed
- Docker installed
- 4GB RAM allocated to Minikube
- 2 CPU cores allocated to Minikube

## 💡 Tips

- Keep `monitor.sh` running in one terminal while testing
- Check both logs and metadata table to see full picture
- StatefulSet ensures predictable naming (consumer-0, consumer-1, etc.)
- LocalStack data persists only while pod is running

## 🐛 Common Issues

1. **Pods not starting**: Check `kubectl describe pod <name>`
2. **Image pull errors**: Run `eval $(minikube docker-env)` and rebuild
3. **LocalStack not ready**: Check logs with `kubectl logs deployment/localstack`
4. **RBAC errors**: Verify with `kubectl get role,rolebinding`

## 🚀 Next Steps

1. Run through all test scenarios
2. Experiment with different shard/worker combinations
3. Integrate with your actual consumer application
4. Adapt for production use (replace LocalStack with real AWS)
5. Add monitoring/alerting

## 📞 Support

For issues or questions:
- Check the troubleshooting section in README.md
- Review logs with `kubectl logs <pod-name>`
- Examine pod events with `kubectl describe pod <pod-name>`

---

**Environment Ready! Start with: `cd k8s/test && ./setup.sh`** 🚀

