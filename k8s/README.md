# KDS Lease Manager - Kubernetes Environment

Complete production-ready Kubernetes deployment for the KDS Lease Manager with Helm, LocalStack testing, and comprehensive automation.

## 🚀 Quick Start

```bash
# One command setup
make setup

# Check status
make status

# Monitor
make monitor
```

## 📁 Structure

```
k8s/
├── Makefile                    # ⭐ Main automation
├── README.md                   # This file
├── execution_details.md        # Detailed walkthrough
├── kds_lease_manager.go        # Production implementation
│
├── docs/                       # 📚 Documentation
│   ├── README.md              # Complete guide
│   ├── QUICK_REFERENCE.md     # Command cheat sheet
│   ├── FLOW_DIAGRAM.md        # Visual diagrams
│   └── SUMMARY.md             # Summary
│
├── scripts/                    # 🔧 Automation scripts
│   ├── setup.sh
│   ├── cleanup.sh
│   ├── test-scale-shards.sh
│   ├── test-scale-workers.sh
│   └── monitor.sh
│
├── helm/                       # ⚡ Helm charts
│   └── kds-lease-manager/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
│
└── test/                       # 🧪 Test application
    └── test-consumer/
```

## 📖 Documentation

| File | Description |
|------|-------------|
| **[docs/README.md](docs/README.md)** | Main documentation - start here! |
| **[docs/QUICK_REFERENCE.md](docs/QUICK_REFERENCE.md)** | Command cheat sheet |
| **[docs/FLOW_DIAGRAM.md](docs/FLOW_DIAGRAM.md)** | Visual flow diagrams |
| **[execution_details.md](execution_details.md)** | Step-by-step theoretical walkthrough |
| **[Makefile](Makefile)** | All automation commands |

## 🎯 Common Commands

### Setup & Deploy
```bash
make setup              # Complete setup
make deploy             # Deploy only
make build              # Build image
```

### Testing
```bash
make test-shards N=60   # Test shard scaling
make test-workers N=5   # Test worker scaling
make test-all           # Run all tests
```

### Monitoring
```bash
make monitor            # Real-time monitoring
make logs               # View all logs
make status             # Check status
make metadata           # Query DynamoDB
```

### Cleanup
```bash
make clean              # Remove resources
make restart            # Clean + redeploy
```

### Help
```bash
make help               # Show all commands
make docs               # Show documentation locations
```

## 🏗️ Architecture

```
┌─────────────────────────────────────────┐
│      Kubernetes (kds-test namespace)     │
│                                          │
│  ┌─────────────┐                        │
│  │ LocalStack  │ (Kinesis + DynamoDB)   │
│  └──────┬──────┘                        │
│         │                                │
│  ┌──────┴─────┬──────────┬──────────┐  │
│  │ Consumer-0 │Consumer-1│Consumer-2│  │
│  │ (StatefulSet)                      │  │
│  └────────────────────────────────────┘  │
│                                          │
│  Managed by: Helm Chart                 │
│  Automated by: Makefile                 │
└─────────────────────────────────────────┘
```

## ⚙️ Configuration

Edit `helm/kds-lease-manager/values.yaml`:

```yaml
namespace: kds-test

consumer:
  replicaCount: 3              # Number of workers
  stream:
    name: test-stream
    initialShardCount: 30      # Initial shards
  app:
    enableDynamicMaxLeases: true
```

## 🧪 Test Scenarios

### Scenario 1: Initial Setup (30 shards, 3 workers)
```bash
make setup
make metadata
# Expected: max_leases_per_worker = 10
```

### Scenario 2: Scale Shards (30 → 60)
```bash
make test-shards N=60
make metadata
# Expected: max_leases_per_worker = 20
```

### Scenario 3: Scale Workers (3 → 5)
```bash
make test-workers N=5
make metadata
# Expected: max_leases_per_worker = 6
```

### Scenario 4: Test 80 Limit
```bash
make test-shards N=300
# Expected: max_leases_per_worker = 80 (capped)
```

## 📝 Formula

```
max_leases_per_worker = min(80, ceil(shard_count / worker_count))
```

## 🎓 Key Features

✅ **Namespace Isolation** - All resources in `kds-test` namespace  
✅ **Helm Deployment** - Easy configuration management  
✅ **Makefile Automation** - Simple command interface  
✅ **StatefulSet** - Stable pod identities  
✅ **RBAC** - Kubernetes API access for worker count  
✅ **Dynamic Calculation** - Auto-adjusts to config changes  
✅ **LocalStack** - AWS simulation for testing  
✅ **Real-time Monitoring** - Live metadata tracking  

## 🐛 Troubleshooting

```bash
# Check status
make status

# View logs
make logs

# Describe resources
kubectl get all -n kds-test
kubectl describe pod <pod-name> -n kds-test

# Verify prerequisites
make verify

# Full documentation
See docs/README.md
```

## 📚 Next Steps

1. **Read**: [docs/README.md](docs/README.md) for complete documentation
2. **Quick Reference**: [docs/QUICK_REFERENCE.md](docs/QUICK_REFERENCE.md) for commands
3. **Understand**: [execution_details.md](execution_details.md) for theory
4. **Deploy**: Run `make setup`
5. **Test**: Run `make test-all`
6. **Monitor**: Run `make monitor`

## 🚧 Prerequisites

- Minikube
- kubectl
- Docker
- Helm v3+

Verify with: `make verify`

## 💡 Tips

- Run `make help` to see all available commands
- Keep `make monitor` running in a separate terminal
- All resources are in the `kds-test` namespace
- Use `NAMESPACE=custom make deploy` for different namespace

---

**Getting Started:** `make setup` then `make status` 🚀

**Full Documentation:** [docs/README.md](docs/README.md)

**Quick Reference:** [docs/QUICK_REFERENCE.md](docs/QUICK_REFERENCE.md)

