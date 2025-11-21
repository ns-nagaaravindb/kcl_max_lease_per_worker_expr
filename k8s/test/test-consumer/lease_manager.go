package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	MaxLeasePerWorkerLimit = 80 // Maximum number of leases a single worker can handle
)

// LeaseMetadata represents the metadata stored in DynamoDB for a worker
type LeaseMetadata struct {
	WorkerID           string    `dynamodbav:"worker_id"`
	MaxLeasesPerWorker int       `dynamodbav:"max_leases_per_worker"`
	StreamName         string    `dynamodbav:"stream_name"`
	AppName            string    `dynamodbav:"app_name"`
	LastUpdateTime     time.Time `dynamodbav:"last_update_time"`
	ShardCount         int       `dynamodbav:"shard_count"`
	WorkerCount        int       `dynamodbav:"worker_count"`
}

// KinesisAPIForLease defines the Kinesis operations needed for lease management
type KinesisAPIForLease interface {
	ListShards(ctx context.Context, params *kinesis.ListShardsInput, optFns ...func(*kinesis.Options)) (*kinesis.ListShardsOutput, error)
}

// DynamoDBAPIForLease defines the DynamoDB operations needed for lease management
type DynamoDBAPIForLease interface {
	CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

// KDSLeaseManager manages the calculation and storage of max leases per worker
type KDSLeaseManager struct {
	region         string
	streamName     string
	appName        string
	workerID       string
	kinesisClient  KinesisAPIForLease
	dynamodbClient DynamoDBAPIForLease
	metadataTable  string
	k8sClient      *kubernetes.Clientset
}

// NewKDSLeaseManager creates a new lease manager
func NewKDSLeaseManager(ctx context.Context, region, streamName, appName, workerID, endpoint string) (*KDSLeaseManager, error) {
	// Load AWS configuration
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	if endpoint != "" {
		opts = append(opts, config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:               endpoint,
					HostnameImmutable: true,
					SigningRegion:     region,
				}, nil
			}),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	kinesisClient := kinesis.NewFromConfig(awsCfg)
	dynamodbClient := dynamodb.NewFromConfig(awsCfg)

	// Create Kubernetes client
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Failed to get in-cluster K8s config, will use fallback methods: %v: %v", err)
	}

	var k8sClient *kubernetes.Clientset
	if k8sConfig != nil {
		k8sClient, err = kubernetes.NewForConfig(k8sConfig)
		if err != nil {
			log.Printf("Failed to create K8s client, will use fallback methods: %v: %v", err)
		}
	}

	metadataTable := appName + "_meta"

	manager := &KDSLeaseManager{
		region:         region,
		streamName:     streamName,
		appName:        appName,
		workerID:       workerID,
		kinesisClient:  kinesisClient,
		dynamodbClient: dynamodbClient,
		metadataTable:  metadataTable,
		k8sClient:      k8sClient,
	}

	return manager, nil
}

// GetShardCount retrieves the number of shards in the KDS stream
func (lm *KDSLeaseManager) GetShardCount(ctx context.Context) (int, error) {
	log.Printf("Getting shard count from KDS stream",
		lm.streamName)

	var shardCount int
	var nextToken *string

	for {
		input := &kinesis.ListShardsInput{
			StreamName: aws.String(lm.streamName),
			NextToken:  nextToken,
		}

		resp, err := lm.kinesisClient.ListShards(ctx, input)
		if err != nil {
			return 0, fmt.Errorf("failed to list shards: %w", err)
		}

		// Count only active shards (those without EndingSequenceNumber)
		for _, shard := range resp.Shards {
			if shard.SequenceNumberRange.EndingSequenceNumber == nil {
				shardCount++
			}
		}

		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}

	log.Printf("Retrieved shard count from KDS",
		lm.streamName,
		shardCount)

	return shardCount, nil
}

// GetWorkerCount retrieves the number of pods/workers in the deployment or statefulset
func (lm *KDSLeaseManager) GetWorkerCount(ctx context.Context) (int, error) {
	log.Printf("Getting worker count from Kubernetes")

	// First, try to get from environment variable (for testing or manual configuration)
	if workerCountEnv := os.Getenv("KDS_WORKER_COUNT"); workerCountEnv != "" {
		count, err := strconv.Atoi(workerCountEnv)
		if err == nil && count > 0 {
			log.Printf("Using worker count from environment variable",
				count)
			return count, nil
		}
	}

	// If K8s client is not available, use default
	if lm.k8sClient == nil {
		log.Printf("WARN: K8s client not available, using default worker count of 1")
		return 1, nil
	}

	// Get current pod's name from HOSTNAME (automatically set in K8s)
	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		log.Printf("WARN: HOSTNAME not set, cannot determine pod name, using default worker count of 1")
		return 1, nil
	}

	// Get current namespace
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		// Try to read from service account namespace file (standard location in K8s)
		namespaceBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err == nil {
			namespace = string(namespaceBytes)
			log.Printf("Read namespace from service account: %v: %v", namespace)
		} else {
			namespace = "default"
			log.Printf("WARN: Could not determine namespace, using default")
		}
	}

	// Get the current pod
	pod, err := lm.k8sClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		log.Printf("WARN: Failed to get pod info, using default worker count of 1",
			err,
			podName,
			namespace)
		return 1, nil
	}

	// Find the owner reference (could be ReplicaSet, StatefulSet, etc.)
	if len(pod.OwnerReferences) == 0 {
		log.Printf("WARN: Pod has no owner references, using default worker count of 1",
			podName)
		return 1, nil
	}

	// Check each owner reference
	for _, owner := range pod.OwnerReferences {
		switch owner.Kind {
		case "StatefulSet":
			statefulset, err := lm.k8sClient.AppsV1().StatefulSets(namespace).Get(ctx, owner.Name, metav1.GetOptions{})
			if err == nil && statefulset.Spec.Replicas != nil {
				workerCount := int(*statefulset.Spec.Replicas)
				log.Printf("Retrieved worker count from StatefulSet (via pod owner)",
					owner.Name,
					podName,
					workerCount)
				return workerCount, nil
			}
			log.Printf("WARN: Failed to get statefulset info: %v: %v", err)

		case "ReplicaSet":
			replicaset, err := lm.k8sClient.AppsV1().ReplicaSets(namespace).Get(ctx, owner.Name, metav1.GetOptions{})
			if err == nil && replicaset.Spec.Replicas != nil {
				// ReplicaSet is likely owned by a Deployment, but we can use its replica count
				workerCount := int(*replicaset.Spec.Replicas)

				// Try to find the parent Deployment for better logging
				deploymentName := ""
				if len(replicaset.OwnerReferences) > 0 {
					for _, rsOwner := range replicaset.OwnerReferences {
						if rsOwner.Kind == "Deployment" {
							deploymentName = rsOwner.Name
							break
						}
					}
				}

				if deploymentName != "" {
					log.Printf("Retrieved worker count from Deployment (via pod -> replicaset -> deployment)",
						deploymentName,
						owner.Name,
						podName,
						workerCount)
				} else {
					log.Printf("Retrieved worker count from ReplicaSet (via pod owner)",
						owner.Name,
						podName,
						workerCount)
				}
				return workerCount, nil
			}
			log.Printf("WARN: Failed to get replicaset info: %v: %v", err)
		}
	}

	// Fallback
	log.Printf("WARN: Unable to determine worker count from pod owners, using default of 1",
		podName)
	return 1, nil
}

// CalculateMaxLeasesPerWorker calculates the maximum number of leases per worker
// Formula: min(80, ceil(shardCount / workerCount))
func (lm *KDSLeaseManager) CalculateMaxLeasesPerWorker(shardCount, workerCount int) int {
	if workerCount <= 0 {
		workerCount = 1
	}

	// Calculate shards per worker
	shardsPerWorker := int(math.Ceil(float64(shardCount) / float64(workerCount)))

	// Apply the limit of 80
	maxLeases := shardsPerWorker
	if maxLeases > MaxLeasePerWorkerLimit {
		maxLeases = MaxLeasePerWorkerLimit
	}

	log.Printf("Calculated max leases per worker",
		shardCount,
		workerCount,
		shardsPerWorker,
		maxLeases)

	return maxLeases
}

// InitializeMetadataTable creates the metadata table if it doesn't exist
func (lm *KDSLeaseManager) InitializeMetadataTable(ctx context.Context) error {
	log.Printf("Initializing metadata table: %v: %v", lm.metadataTable)

	// Check if table exists
	_, err := lm.dynamodbClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(lm.metadataTable),
	})

	if err == nil {
		log.Printf("Metadata table already exists: %v: %v", lm.metadataTable)
		return nil
	}

	// Create table
	input := &dynamodb.CreateTableInput{
		TableName: aws.String(lm.metadataTable),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("worker_id"),
				KeyType:       types.KeyTypeHash,
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("worker_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	}

	_, err = lm.dynamodbClient.CreateTable(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	// Wait for table to be active (simple retry loop)
	waitTimeout := 2 * time.Minute
	waitStart := time.Now()
	for {
		desc, err := lm.dynamodbClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(lm.metadataTable),
		})
		if err == nil && desc.Table != nil && desc.Table.TableStatus == types.TableStatusActive {
			log.Printf("Metadata table created successfully: %v: %v", lm.metadataTable)
			return nil
		}
		if time.Since(waitStart) > waitTimeout {
			return fmt.Errorf("timeout waiting for metadata table to be active")
		}
		time.Sleep(2 * time.Second)
	}
}

// SaveMetadata saves the lease metadata to DynamoDB
func (lm *KDSLeaseManager) SaveMetadata(ctx context.Context, metadata *LeaseMetadata) error {
	metadata.LastUpdateTime = time.Now()

	item := map[string]types.AttributeValue{
		"worker_id":             &types.AttributeValueMemberS{Value: metadata.WorkerID},
		"max_leases_per_worker": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metadata.MaxLeasesPerWorker)},
		"stream_name":           &types.AttributeValueMemberS{Value: metadata.StreamName},
		"app_name":              &types.AttributeValueMemberS{Value: metadata.AppName},
		"last_update_time":      &types.AttributeValueMemberS{Value: metadata.LastUpdateTime.Format(time.RFC3339)},
		"shard_count":           &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metadata.ShardCount)},
		"worker_count":          &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metadata.WorkerCount)},
	}

	_, err := lm.dynamodbClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(lm.metadataTable),
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("failed to save metadata to DynamoDB: %w", err)
	}

	log.Printf("Saved lease metadata to DynamoDB",
		metadata.WorkerID,
		metadata.MaxLeasesPerWorker,
		lm.metadataTable)

	return nil
}

// GetMetadata retrieves the lease metadata for this worker from DynamoDB
func (lm *KDSLeaseManager) GetMetadata(ctx context.Context) (*LeaseMetadata, error) {
	result, err := lm.dynamodbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(lm.metadataTable),
		Key: map[string]types.AttributeValue{
			"worker_id": &types.AttributeValueMemberS{Value: lm.workerID},
		},
		ConsistentRead: aws.Bool(true),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get metadata from DynamoDB: %w", err)
	}

	if result.Item == nil {
		return nil, nil // No metadata exists yet
	}

	metadata := &LeaseMetadata{
		WorkerID:   lm.workerID,
		StreamName: lm.streamName,
		AppName:    lm.appName,
	}

	if val, ok := result.Item["max_leases_per_worker"]; ok {
		if numVal, ok := val.(*types.AttributeValueMemberN); ok {
			maxLeases, _ := strconv.Atoi(numVal.Value)
			metadata.MaxLeasesPerWorker = maxLeases
		}
	}

	if val, ok := result.Item["shard_count"]; ok {
		if numVal, ok := val.(*types.AttributeValueMemberN); ok {
			shardCount, _ := strconv.Atoi(numVal.Value)
			metadata.ShardCount = shardCount
		}
	}

	if val, ok := result.Item["worker_count"]; ok {
		if numVal, ok := val.(*types.AttributeValueMemberN); ok {
			workerCount, _ := strconv.Atoi(numVal.Value)
			metadata.WorkerCount = workerCount
		}
	}

	return metadata, nil
}

// getCoordinatorKey returns the coordinator key for this deployment/statefulset
func (lm *KDSLeaseManager) getCoordinatorKey() string {
	// Use app_name as coordinator key - all pods in same deployment/statefulset share the same app_name
	return lm.appName + "_coordinator"
}

// GetCoordinatorMetadata retrieves the coordinator metadata (computed max leases)
func (lm *KDSLeaseManager) GetCoordinatorMetadata(ctx context.Context) (*LeaseMetadata, error) {
	coordinatorKey := lm.getCoordinatorKey()
	result, err := lm.dynamodbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(lm.metadataTable),
		Key: map[string]types.AttributeValue{
			"worker_id": &types.AttributeValueMemberS{Value: coordinatorKey},
		},
		ConsistentRead: aws.Bool(true),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get coordinator metadata from DynamoDB: %w", err)
	}

	if result.Item == nil {
		return nil, nil // No coordinator metadata exists yet
	}

	metadata := &LeaseMetadata{
		WorkerID:   coordinatorKey,
		StreamName: lm.streamName,
		AppName:    lm.appName,
	}

	if val, ok := result.Item["max_leases_per_worker"]; ok {
		if numVal, ok := val.(*types.AttributeValueMemberN); ok {
			maxLeases, _ := strconv.Atoi(numVal.Value)
			metadata.MaxLeasesPerWorker = maxLeases
		}
	}

	if val, ok := result.Item["shard_count"]; ok {
		if numVal, ok := val.(*types.AttributeValueMemberN); ok {
			shardCount, _ := strconv.Atoi(numVal.Value)
			metadata.ShardCount = shardCount
		}
	}

	if val, ok := result.Item["worker_count"]; ok {
		if numVal, ok := val.(*types.AttributeValueMemberN); ok {
			workerCount, _ := strconv.Atoi(numVal.Value)
			metadata.WorkerCount = workerCount
		}
	}

	return metadata, nil
}

// UpdateCoordinatorMetadata updates existing coordinator metadata with new values
// Uses conditional update to ensure the old values match (prevents race conditions)
func (lm *KDSLeaseManager) UpdateCoordinatorMetadata(ctx context.Context, newMetadata *LeaseMetadata, expectedShardCount, expectedWorkerCount int) error {
	coordinatorKey := lm.getCoordinatorKey()
	newMetadata.WorkerID = coordinatorKey
	newMetadata.LastUpdateTime = time.Now()

	item := map[string]types.AttributeValue{
		"worker_id":             &types.AttributeValueMemberS{Value: newMetadata.WorkerID},
		"max_leases_per_worker": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", newMetadata.MaxLeasesPerWorker)},
		"stream_name":           &types.AttributeValueMemberS{Value: newMetadata.StreamName},
		"app_name":              &types.AttributeValueMemberS{Value: newMetadata.AppName},
		"last_update_time":      &types.AttributeValueMemberS{Value: newMetadata.LastUpdateTime.Format(time.RFC3339)},
		"shard_count":           &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", newMetadata.ShardCount)},
		"worker_count":          &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", newMetadata.WorkerCount)},
	}

	// Use conditional update: only update if shard_count and worker_count still match expected values
	// This prevents race conditions when multiple workers restart simultaneously
	conditionExpr := "shard_count = :expected_shard_count AND worker_count = :expected_worker_count"
	exprAttrValues := map[string]types.AttributeValue{
		":expected_shard_count":  &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", expectedShardCount)},
		":expected_worker_count": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", expectedWorkerCount)},
	}

	_, err := lm.dynamodbClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:                 aws.String(lm.metadataTable),
		Item:                      item,
		ConditionExpression:       aws.String(conditionExpr),
		ExpressionAttributeValues: exprAttrValues,
	})

	if err != nil {
		// Check if it's a conditional check failed error (another worker already updated it)
		var condCheckErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckErr) {
			log.Printf("Another worker already updated coordinator metadata with different values",
				coordinatorKey)
			return nil // Not an error - another worker successfully updated
		}
		return fmt.Errorf("failed to update coordinator metadata: %w", err)
	}

	log.Printf("Successfully updated coordinator metadata",
		coordinatorKey,
		newMetadata.MaxLeasesPerWorker,
		newMetadata.ShardCount,
		newMetadata.WorkerCount)
	return nil
}

// TryCreateCoordinatorMetadata attempts to create coordinator metadata using conditional write
// Returns true if this worker successfully became the coordinator, false otherwise
func (lm *KDSLeaseManager) TryCreateCoordinatorMetadata(ctx context.Context, metadata *LeaseMetadata) (bool, error) {
	coordinatorKey := lm.getCoordinatorKey()
	metadata.WorkerID = coordinatorKey
	metadata.LastUpdateTime = time.Now()

	item := map[string]types.AttributeValue{
		"worker_id":             &types.AttributeValueMemberS{Value: metadata.WorkerID},
		"max_leases_per_worker": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metadata.MaxLeasesPerWorker)},
		"stream_name":           &types.AttributeValueMemberS{Value: metadata.StreamName},
		"app_name":              &types.AttributeValueMemberS{Value: metadata.AppName},
		"last_update_time":      &types.AttributeValueMemberS{Value: metadata.LastUpdateTime.Format(time.RFC3339)},
		"shard_count":           &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metadata.ShardCount)},
		"worker_count":          &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metadata.WorkerCount)},
	}

	// Use conditional write: only create if item doesn't exist (attribute_not_exists)
	_, err := lm.dynamodbClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(lm.metadataTable),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(worker_id)"),
	})

	if err != nil {
		// Check if it's a conditional check failed error (another worker already created it)
		var condCheckErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckErr) {
			log.Printf("Another worker already created coordinator metadata, will use existing value",
				coordinatorKey)
			return false, nil
		}
		return false, fmt.Errorf("failed to create coordinator metadata: %w", err)
	}

	log.Printf("Successfully became coordinator and created metadata",
		coordinatorKey,
		metadata.MaxLeasesPerWorker)
	return true, nil
}

// InitializeMaxLeasesPerWorker is the main function that orchestrates the entire process
// Only one worker per deployment/statefulset computes the value, others reuse it from DynamoDB
// If shard count or worker count changes, it automatically recalculates and updates the coordinator
func (lm *KDSLeaseManager) InitializeMaxLeasesPerWorker(ctx context.Context) (int, error) {
	log.Printf("Initializing max leases per worker",
		lm.streamName,
		lm.appName,
		lm.workerID)

	// 1. Initialize metadata table
	if err := lm.InitializeMetadataTable(ctx); err != nil {
		return 0, fmt.Errorf("failed to initialize metadata table: %w", err)
	}

	// 2. Get current shard count and worker count
	currentShardCount, err := lm.GetShardCount(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get shard count: %w", err)
	}

	currentWorkerCount, err := lm.GetWorkerCount(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get worker count: %w", err)
	}

	log.Printf("Retrieved current system state",
		currentShardCount,
		currentWorkerCount)

	// 3. Check if coordinator metadata already exists
	coordinatorMetadata, err := lm.GetCoordinatorMetadata(ctx)
	if err != nil {
		log.Printf("WARN: Failed to get coordinator metadata, will attempt to compute: %v: %v", err)
	} else if coordinatorMetadata != nil {
		// Coordinator metadata exists - check if shard/worker counts have changed
		configChanged := coordinatorMetadata.ShardCount != currentShardCount ||
			coordinatorMetadata.WorkerCount != currentWorkerCount

		if configChanged {
			log.Printf("Detected configuration change, recalculating max leases per worker",
				coordinatorMetadata.ShardCount,
				currentShardCount,
				coordinatorMetadata.WorkerCount,
				currentWorkerCount,
				coordinatorMetadata.MaxLeasesPerWorker)

			// Calculate new max leases per worker
			newMaxLeasesPerWorker := lm.CalculateMaxLeasesPerWorker(currentShardCount, currentWorkerCount)

			// Try to update coordinator metadata (race-safe)
			updatedMetadata := &LeaseMetadata{
				WorkerID:           lm.getCoordinatorKey(),
				MaxLeasesPerWorker: newMaxLeasesPerWorker,
				StreamName:         lm.streamName,
				AppName:            lm.appName,
				ShardCount:         currentShardCount,
				WorkerCount:        currentWorkerCount,
			}

			// Attempt to update - if another worker updates first, we'll read their value
			err = lm.UpdateCoordinatorMetadata(ctx, updatedMetadata, coordinatorMetadata.ShardCount, coordinatorMetadata.WorkerCount)
			if err != nil {
				log.Printf("WARN: Failed to update coordinator metadata, will read latest value",
					err)
				// Read the latest value (another worker may have updated it)
				coordinatorMetadata, err = lm.GetCoordinatorMetadata(ctx)
				if err != nil {
					return 0, fmt.Errorf("failed to get updated coordinator metadata: %w", err)
				}
			} else {
				log.Printf("Successfully updated coordinator metadata with new configuration",
					newMaxLeasesPerWorker)
				coordinatorMetadata = updatedMetadata
			}
		} else {
			log.Printf("Configuration unchanged, using existing coordinator metadata",
				coordinatorMetadata.MaxLeasesPerWorker,
				coordinatorMetadata.ShardCount,
				coordinatorMetadata.WorkerCount)
		}

		// Save this worker's metadata for tracking
		workerMetadata := &LeaseMetadata{
			WorkerID:           lm.workerID,
			MaxLeasesPerWorker: coordinatorMetadata.MaxLeasesPerWorker,
			StreamName:         lm.streamName,
			AppName:            lm.appName,
			ShardCount:         coordinatorMetadata.ShardCount,
			WorkerCount:        coordinatorMetadata.WorkerCount,
		}
		if err := lm.SaveMetadata(ctx, workerMetadata); err != nil {
			log.Printf("WARN: Failed to save worker metadata, continuing with coordinator value: %v: %v", err)
		}

		return coordinatorMetadata.MaxLeasesPerWorker, nil
	}

	// 3. No coordinator exists yet - this worker will attempt to become coordinator
	log.Printf("No coordinator metadata found, attempting to become coordinator and compute value")

	// 4. Calculate max leases per worker
	maxLeasesPerWorker := lm.CalculateMaxLeasesPerWorker(currentShardCount, currentWorkerCount)

	// 5. Try to create coordinator metadata (only one worker will succeed)
	coordinatorMetadata = &LeaseMetadata{
		WorkerID:           lm.getCoordinatorKey(),
		MaxLeasesPerWorker: maxLeasesPerWorker,
		StreamName:         lm.streamName,
		AppName:            lm.appName,
		ShardCount:         currentShardCount,
		WorkerCount:        currentWorkerCount,
	}

	becameCoordinator, err := lm.TryCreateCoordinatorMetadata(ctx, coordinatorMetadata)
	if err != nil {
		return 0, fmt.Errorf("failed to create coordinator metadata: %w", err)
	}

	if !becameCoordinator {
		// Another worker became coordinator, read the value they computed
		coordinatorMetadata, err = lm.GetCoordinatorMetadata(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to get coordinator metadata after creation attempt: %w", err)
		}
		if coordinatorMetadata == nil {
			return 0, fmt.Errorf("coordinator metadata not found after creation attempt")
		}
		maxLeasesPerWorker = coordinatorMetadata.MaxLeasesPerWorker
		log.Printf("Using coordinator metadata created by another worker",
			maxLeasesPerWorker)
	} else {
		log.Printf("Successfully computed and stored coordinator metadata",
			maxLeasesPerWorker,
			currentShardCount,
			currentWorkerCount)
	}

	// 6. Save this worker's metadata for tracking
	workerMetadata := &LeaseMetadata{
		WorkerID:           lm.workerID,
		MaxLeasesPerWorker: maxLeasesPerWorker,
		StreamName:         lm.streamName,
		AppName:            lm.appName,
		ShardCount:         currentShardCount,
		WorkerCount:        currentWorkerCount,
	}
	if err := lm.SaveMetadata(ctx, workerMetadata); err != nil {
		log.Printf("WARN: Failed to save worker metadata, but continuing with computed value: %v: %v", err)
	}

	return maxLeasesPerWorker, nil
}

// ListAllWorkerMetadata retrieves metadata for all workers in the group
func (lm *KDSLeaseManager) ListAllWorkerMetadata(ctx context.Context) ([]*LeaseMetadata, error) {
	result, err := lm.dynamodbClient.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(lm.metadataTable),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan metadata table: %w", err)
	}

	var metadataList []*LeaseMetadata
	for _, item := range result.Items {
		metadata := &LeaseMetadata{}

		if val, ok := item["worker_id"]; ok {
			if strVal, ok := val.(*types.AttributeValueMemberS); ok {
				metadata.WorkerID = strVal.Value
			}
		}

		if val, ok := item["max_leases_per_worker"]; ok {
			if numVal, ok := val.(*types.AttributeValueMemberN); ok {
				maxLeases, _ := strconv.Atoi(numVal.Value)
				metadata.MaxLeasesPerWorker = maxLeases
			}
		}

		if val, ok := item["stream_name"]; ok {
			if strVal, ok := val.(*types.AttributeValueMemberS); ok {
				metadata.StreamName = strVal.Value
			}
		}

		if val, ok := item["app_name"]; ok {
			if strVal, ok := val.(*types.AttributeValueMemberS); ok {
				metadata.AppName = strVal.Value
			}
		}

		if val, ok := item["shard_count"]; ok {
			if numVal, ok := val.(*types.AttributeValueMemberN); ok {
				shardCount, _ := strconv.Atoi(numVal.Value)
				metadata.ShardCount = shardCount
			}
		}

		if val, ok := item["worker_count"]; ok {
			if numVal, ok := val.(*types.AttributeValueMemberN); ok {
				workerCount, _ := strconv.Atoi(numVal.Value)
				metadata.WorkerCount = workerCount
			}
		}

		metadataList = append(metadataList, metadata)
	}

	return metadataList, nil
}

