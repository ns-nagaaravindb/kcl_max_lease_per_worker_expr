.PHONY: help build start stop clean producer producer-20 consumer-pod1 consumer-pod2 consumer-pod3 consumer-pod4 consumer-pod5 verify split monitor view-dist analyze k8s-deploy k8s-scale k8s-delete

help:
	@echo "=========================================="
	@echo "Mohan's KCL Rebalancing Experiment"
	@echo "=========================================="
	@echo ""
	@echo "📦 Setup Commands:"
	@echo "  make build          - Build producer and consumer"
	@echo "  make start          - Start LocalStack with 20 shards"
	@echo "  make stop           - Stop LocalStack"
	@echo "  make clean          - Clean up all artifacts"
	@echo ""
	@echo "🚀 Producer Commands:"
	@echo "  make producer       - Run producer for 20 shards"
	@echo ""
	@echo "👥 Consumer Commands (Local):"
	@echo "  make consumer-pod1  - Run consumer pod 1"
	@echo "  make consumer-pod2  - Run consumer pod 2"
	@echo "  make consumer-pod3  - Run consumer pod 3"
	@echo "  make consumer-pod4  - Run consumer pod 4 (scale-up)"
	@echo "  make consumer-pod5  - Run consumer pod 5 (scale-up)"
	@echo ""
	@echo "🔄 Shard Operations:"
	@echo "  make verify         - Verify stream and shard count"
	@echo "  make split          - Split shards (20 → 30)"
	@echo ""
	@echo "📊 Monitoring Commands:"
	@echo "  make monitor        - Monitor lease distribution (live)"
	@echo "  make view-dist      - View current distribution snapshot"
	@echo "  make analyze        - Analyze rebalancing performance"
	@echo ""
	@echo "☸️  Kubernetes Commands:"
	@echo "  make k8s-deploy     - Deploy to Kubernetes (3 pods)"
	@echo "  make k8s-scale      - Scale to 5 pods"
	@echo "  make k8s-delete     - Delete Kubernetes resources"
	@echo ""

# Build
build:
	@echo "🔨 Building producer and consumer..."
	@cd producer && go build -o ../bin/producer producer.go
	@cd consumer && go build -o ../bin/enhanced-consumer enhanced_consumer.go
	@echo "✅ Build complete!"

# LocalStack operations
start:
	@echo "🚀 Starting LocalStack with 20 shards..."
	@docker-compose -f docker-compose-20.yml up -d
	@echo "⏳ Waiting for initialization..."
	@sleep 15
	@echo "✅ LocalStack started!"
	@./scripts/verify-stream.sh

stop:
	@echo "🛑 Stopping LocalStack..."
	@docker-compose -f docker-compose-20.yml down
	@echo "✅ LocalStack stopped!"

clean:
	@echo "🧹 Cleaning up..."
	@rm -rf bin/
	@rm -rf lease-data-*.json
	@docker-compose -f docker-compose-20.yml down -v
	@echo "✅ Cleanup complete!"

# Producer
producer:
	@echo "🚀 Starting Producer for 20 shards..."
	@cd producer && go run producer.go

# Consumers
consumer-pod1:
	@echo "🚀 Starting Consumer Pod 1..."
	@cd consumer && CONFIG_FILE=../config/config-pod1.yaml go run enhanced_consumer.go

consumer-pod2:
	@echo "🚀 Starting Consumer Pod 2..."
	@cd consumer && CONFIG_FILE=../config/config-pod2.yaml go run enhanced_consumer.go

consumer-pod3:
	@echo "🚀 Starting Consumer Pod 3..."
	@cd consumer && CONFIG_FILE=../config/config-pod3.yaml go run enhanced_consumer.go

consumer-pod4:
	@echo "🚀 Starting Consumer Pod 4 (Scale-up)..."
	@cd consumer && CONFIG_FILE=../config/config-pod4.yaml go run enhanced_consumer.go

consumer-pod5:
	@echo "🚀 Starting Consumer Pod 5 (Scale-up)..."
	@cd consumer && CONFIG_FILE=../config/config-pod5.yaml go run enhanced_consumer.go

consumer-pod6:
	@echo "🚀 Starting Consumer Pod 6 (Scale-up)..."
	@cd consumer && CONFIG_FILE=../config/config-pod6.yaml go run enhanced_consumer.go

consumer-pod7:
	@echo "🚀 Starting Consumer Pod 7 (Scale-up)..."
	@cd consumer && CONFIG_FILE=../config/config-pod7.yaml go run enhanced_consumer.go

# Shard operations
verify:
	@./scripts/verify-stream.sh

split:
	@./scripts/split-shards.sh

# Monitoring
monitor:
	@./scripts/monitor-leases.sh

view-dist:
	@./scripts/view-distribution.sh

analyze:
	@./scripts/analyze-rebalancing.sh



# Quick start guide
quickstart:
	@echo "=========================================="
	@echo "🚀 Quick Start Guide"
	@echo "=========================================="
	@echo ""
	@echo "1️⃣  Start LocalStack and create stream:"
	@echo "   make start"
	@echo ""
	@echo "2️⃣  In separate terminals, start 3 consumers:"
	@echo "   Terminal 2: make consumer-pod1"
	@echo "   Terminal 3: make consumer-pod2"
	@echo "   Terminal 4: make consumer-pod3"
	@echo ""
	@echo "3️⃣  In another terminal, start producer:"
	@echo "   Terminal 5: make producer"
	@echo ""
	@echo "4️⃣  Monitor lease distribution:"
	@echo "   Terminal 6: make monitor"
	@echo ""
	@echo "5️⃣  Split shards (20 → 30):"
	@echo "   make split"
	@echo ""
	@echo "6️⃣  Scale up consumers (start in new terminals):"
	@echo "   Terminal 7: make consumer-pod4"
	@echo "   Terminal 8: make consumer-pod5"
	@echo ""
	@echo "7️⃣  Observe rebalancing:"
	@echo "   Watch the monitor output in Terminal 6"
	@echo "   Or run: make analyze"
	@echo ""
	@echo "=========================================="

