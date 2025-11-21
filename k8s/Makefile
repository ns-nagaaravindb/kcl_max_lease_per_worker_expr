.PHONY: help setup build deploy install test-shards test-workers monitor logs status clean delete-minikube helm-lint helm-template

# Variables
NAMESPACE ?= kds-test
IMAGE_NAME := kds-consumer-test
IMAGE_TAG := latest
HELM_CHART := ./helm/kds-lease-manager
TEST_APP_DIR := ./test/test-consumer
SCRIPTS_DIR := ./scripts

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

help: ## Show this help message
	@echo "$(GREEN)KDS Lease Manager - Makefile Commands$(NC)"
	@echo ""
	@echo "$(YELLOW)Setup Commands:$(NC)"
	@echo "  make setup              - Complete setup (minikube + build + deploy)"
	@echo "  make build              - Build Docker image"
	@echo "  make deploy             - Deploy using Helm"
	@echo "  make install            - Alias for deploy"
	@echo ""
	@echo "$(YELLOW)Test Commands:$(NC)"
	@echo "  make test-shards N=60   - Test shard scaling to N shards"
	@echo "  make test-workers N=5   - Test worker scaling to N workers"
	@echo "  make test-all           - Run all tests"
	@echo ""
	@echo "$(YELLOW)Monitoring Commands:$(NC)"
	@echo "  make monitor            - Monitor metadata in real-time"
	@echo "  make logs               - View logs from all workers"
	@echo "  make logs-worker W=0    - View logs from specific worker"
	@echo "  make status             - Show deployment status"
	@echo "  make metadata           - Query DynamoDB metadata table"
	@echo ""
	@echo "$(YELLOW)Cleanup Commands:$(NC)"
	@echo "  make clean              - Remove all resources (keep minikube)"
	@echo "  make delete-minikube    - Delete minikube cluster"
	@echo "  make restart            - Clean and redeploy"
	@echo ""
	@echo "$(YELLOW)Helm Commands:$(NC)"
	@echo "  make helm-lint          - Lint Helm chart"
	@echo "  make helm-template      - Generate Kubernetes manifests"
	@echo "  make helm-upgrade       - Upgrade existing deployment"
	@echo ""
	@echo "$(YELLOW)Variables:$(NC)"
	@echo "  NAMESPACE=$(NAMESPACE)  - Kubernetes namespace"
	@echo ""

setup: ## Complete setup - start minikube, build image, deploy
	@echo "$(GREEN)Starting complete setup...$(NC)"
	@$(SCRIPTS_DIR)/setup.sh

build: ## Build Docker image
	@echo "$(GREEN)Building Docker image...$(NC)"
	@eval $$(minikube docker-env) && \
	cd $(TEST_APP_DIR) && \
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
	@echo "$(GREEN)✅ Image built: $(IMAGE_NAME):$(IMAGE_TAG)$(NC)"

deploy: build ## Deploy using Helm
	@echo "$(GREEN)Deploying with Helm...$(NC)"
	helm upgrade --install kds-lease-manager $(HELM_CHART) \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--wait \
		--timeout 5m
	@echo "$(GREEN)✅ Deployment complete$(NC)"

install: deploy ## Alias for deploy

helm-lint: ## Lint Helm chart
	@echo "$(GREEN)Linting Helm chart...$(NC)"
	helm lint $(HELM_CHART)

helm-template: ## Generate Kubernetes manifests from Helm
	@echo "$(GREEN)Generating Kubernetes manifests...$(NC)"
	helm template kds-lease-manager $(HELM_CHART) \
		--namespace $(NAMESPACE) > /tmp/kds-manifests.yaml
	@echo "$(GREEN)✅ Manifests generated at /tmp/kds-manifests.yaml$(NC)"

helm-upgrade: build ## Upgrade existing Helm deployment
	@echo "$(GREEN)Upgrading Helm release...$(NC)"
	helm upgrade kds-lease-manager $(HELM_CHART) \
		--namespace $(NAMESPACE) \
		--wait \
		--timeout 5m
	@echo "$(GREEN)✅ Upgrade complete$(NC)"

test-shards: ## Test shard scaling (use N=<count>)
ifndef N
	@echo "$(RED)Error: Please specify shard count with N=<count>$(NC)"
	@echo "Example: make test-shards N=60"
	@exit 1
endif
	@echo "$(GREEN)Testing shard scaling to $(N) shards...$(NC)"
	@NAMESPACE=$(NAMESPACE) $(SCRIPTS_DIR)/test-scale-shards.sh $(N)

test-workers: ## Test worker scaling (use N=<count>)
ifndef N
	@echo "$(RED)Error: Please specify worker count with N=<count>$(NC)"
	@echo "Example: make test-workers N=5"
	@exit 1
endif
	@echo "$(GREEN)Testing worker scaling to $(N) workers...$(NC)"
	@NAMESPACE=$(NAMESPACE) $(SCRIPTS_DIR)/test-scale-workers.sh $(N)

test-all: ## Run all tests
	@echo "$(GREEN)Running all tests...$(NC)"
	@echo ""
	@echo "Test 1: Scale shards from 30 to 60"
	@NAMESPACE=$(NAMESPACE) $(SCRIPTS_DIR)/test-scale-shards.sh 60
	@echo ""
	@sleep 5
	@echo "Test 2: Scale workers from 3 to 5"
	@NAMESPACE=$(NAMESPACE) $(SCRIPTS_DIR)/test-scale-workers.sh 5
	@echo ""
	@sleep 5
#   localstock limit .scaling more than 100 shards not possible
# 	@echo "Test 3: Test 80 limit - Scale shards to 300"
# 	@NAMESPACE=$(NAMESPACE) $(SCRIPTS_DIR)/test-scale-shards.sh 300
# 	@echo ""
# 	@echo "$(GREEN)✅ All tests complete$(NC)"

monitor: ## Monitor metadata table in real-time
	@echo "$(GREEN)Starting metadata monitor (Ctrl+C to exit)...$(NC)"
	@NAMESPACE=$(NAMESPACE) $(SCRIPTS_DIR)/monitor.sh

logs: ## View logs from all workers
	@echo "$(GREEN)Streaming logs from all workers (Ctrl+C to exit)...$(NC)"
	kubectl logs -n $(NAMESPACE) -l app=kds-consumer --all-containers=true -f

logs-worker: ## View logs from specific worker (use W=<number>)
ifndef W
	@echo "$(RED)Error: Please specify worker number with W=<number>$(NC)"
	@echo "Example: make logs-worker W=0"
	@exit 1
endif
	@echo "$(GREEN)Streaming logs from worker-$(W)...$(NC)"
	kubectl logs -n $(NAMESPACE) kds-consumer-$(W) -f

status: ## Show deployment status
	@echo "$(GREEN)Deployment Status:$(NC)"
	@echo ""
	@echo "$(YELLOW)Helm Releases:$(NC)"
	@helm list -n $(NAMESPACE) || echo "No releases found"
	@echo ""
	@echo "$(YELLOW)Pods:$(NC)"
	@kubectl get pods -n $(NAMESPACE) -o wide || echo "No pods found"
	@echo ""
	@echo "$(YELLOW)Services:$(NC)"
	@kubectl get svc -n $(NAMESPACE) || echo "No services found"
	@echo ""
	@echo "$(YELLOW)StatefulSets:$(NC)"
	@kubectl get statefulsets -n $(NAMESPACE) || echo "No statefulsets found"
	@echo ""

metadata: ## Query DynamoDB metadata table
	@echo "$(GREEN)Querying metadata table...$(NC)"
	@kubectl exec -n $(NAMESPACE) deployment/localstack -- \
		awslocal dynamodb scan --table-name kds-consumer-app_meta \
		--query 'Items[*].[worker_id.S, max_leases_per_worker.N, shard_count.N, worker_count.N]' \
		--output table || echo "$(RED)Error querying metadata$(NC)"

metadata-coordinator: ## Query coordinator metadata
	@echo "$(GREEN)Querying coordinator metadata...$(NC)"
	@kubectl exec -n $(NAMESPACE) deployment/localstack -- \
		awslocal dynamodb get-item \
		--table-name kds-consumer-app_meta \
		--key '{"worker_id":{"S":"kds-consumer-app_coordinator"}}'

scale-workers: ## Scale workers (use N=<count>)
ifndef N
	@echo "$(RED)Error: Please specify worker count with N=<count>$(NC)"
	@echo "Example: make scale-workers N=5"
	@exit 1
endif
	@echo "$(GREEN)Scaling to $(N) workers...$(NC)"
	kubectl scale statefulset kds-consumer -n $(NAMESPACE) --replicas=$(N)
	@echo "$(GREEN)✅ Scaled to $(N) workers$(NC)"

restart-worker: ## Restart specific worker (use W=<number>)
ifndef W
	@echo "$(RED)Error: Please specify worker number with W=<number>$(NC)"
	@echo "Example: make restart-worker W=0"
	@exit 1
endif
	@echo "$(GREEN)Restarting worker-$(W)...$(NC)"
	kubectl delete pod kds-consumer-$(W) -n $(NAMESPACE)

restart-all: ## Restart all workers
	@echo "$(GREEN)Restarting all workers...$(NC)"
	kubectl delete pods -n $(NAMESPACE) -l app=kds-consumer

clean: ## Remove all resources (keep minikube running)
	@echo "$(GREEN)Cleaning up resources...$(NC)"
	@$(SCRIPTS_DIR)/cleanup.sh
	@echo "$(GREEN)✅ Cleanup complete$(NC)"

delete-minikube: ## Delete minikube cluster completely
	@echo "$(YELLOW)⚠️  This will delete the entire minikube cluster!$(NC)"
	@echo "Press Ctrl+C to cancel, or wait 5 seconds to continue..."
	@sleep 5
	@echo "$(GREEN)Deleting minikube cluster...$(NC)"
	minikube delete
	@echo "$(GREEN)✅ Minikube deleted$(NC)"

restart: clean deploy ## Clean and redeploy
	@echo "$(GREEN)✅ Restart complete$(NC)"

verify: ## Verify prerequisites
	@echo "$(GREEN)Verifying prerequisites...$(NC)"
	@command -v minikube >/dev/null 2>&1 || (echo "$(RED)❌ minikube not installed$(NC)" && exit 1)
	@command -v kubectl >/dev/null 2>&1 || (echo "$(RED)❌ kubectl not installed$(NC)" && exit 1)
	@command -v docker >/dev/null 2>&1 || (echo "$(RED)❌ docker not installed$(NC)" && exit 1)
	@command -v helm >/dev/null 2>&1 || (echo "$(RED)❌ helm not installed$(NC)" && exit 1)
	@echo "$(GREEN)✅ All prerequisites installed$(NC)"

start-minikube: ## Start minikube
	@echo "$(GREEN)Starting minikube...$(NC)"
	minikube start --memory=4096 --cpus=2
	@echo "$(GREEN)✅ Minikube started$(NC)"

stop-minikube: ## Stop minikube
	@echo "$(GREEN)Stopping minikube...$(NC)"
	minikube stop
	@echo "$(GREEN)✅ Minikube stopped$(NC)"

port-forward-localstack: ## Port forward LocalStack to localhost:4566
	@echo "$(GREEN)Port forwarding LocalStack to localhost:4566$(NC)"
	@echo "Press Ctrl+C to stop"
	kubectl port-forward -n $(NAMESPACE) service/localstack 4566:4566

shell-localstack: ## Open shell in LocalStack pod
	kubectl exec -n $(NAMESPACE) -it deployment/localstack -- /bin/bash

shell-worker: ## Open shell in worker pod (use W=<number>)
ifndef W
	@echo "$(RED)Error: Please specify worker number with W=<number>$(NC)"
	@echo "Example: make shell-worker W=0"
	@exit 1
endif
	kubectl exec -n $(NAMESPACE) -it kds-consumer-$(W) -- /bin/sh

docs: ## Show documentation locations
	@echo "$(GREEN)Documentation:$(NC)"
	@echo "  Main README:        docs/README.md"
	@echo "  Quick Reference:    docs/QUICK_REFERENCE.md"
	@echo "  Flow Diagrams:      docs/FLOW_DIAGRAM.md"
	@echo "  Summary:            docs/SUMMARY.md"
	@echo "  Execution Details:  execution_details.md"
	@echo ""
	@echo "$(GREEN)Scripts:$(NC)"
	@echo "  Setup:              scripts/setup.sh"
	@echo "  Cleanup:            scripts/cleanup.sh"
	@echo "  Test Shards:        scripts/test-scale-shards.sh"
	@echo "  Test Workers:       scripts/test-scale-workers.sh"
	@echo "  Monitor:            scripts/monitor.sh"

.DEFAULT_GOAL := help


