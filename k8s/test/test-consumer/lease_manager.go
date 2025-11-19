package main

import (
	"context"
	"errors"
	"fmt"
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
	MaxLeasePerWorkerLimit = 30
)

type LeaseMetadata struct {
	WorkerID           string
	MaxLeasesPerWorker int
	StreamName         string
	AppName            string
	LastUpdateTime     time.Time
	ShardCount         int
	WorkerCount        int
}

type TestLeaseManager struct {
	region         string
	streamName     string
	appName        string
	workerID       string
	kinesisClient  *kinesis.Client
	dynamodbClient *dynamodb.Client
	metadataTable  string
	k8sClient      *kubernetes.Clientset
}

func NewTestLeaseManager(ctx context.Context, region, streamName, appName, workerID, endpoint string) (*TestLeaseManager, error) {
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
		fmt.Printf("Warning: Failed to get in-cluster K8s config: %v\n", err)
	}

	var k8sClient *kubernetes.Clientset
	if k8sConfig != nil {
		k8sClient, err = kubernetes.NewForConfig(k8sConfig)
		if err != nil {
			fmt.Printf("Warning: Failed to create K8s client: %v\n", err)
		}
	}

	return &TestLeaseManager{
		region:         region,
		streamName:     streamName,
		appName:        appName,
		workerID:       workerID,
		kinesisClient:  kinesisClient,
		dynamodbClient: dynamodbClient,
		metadataTable:  appName + "_meta",
		k8sClient:      k8sClient,
	}, nil
}

func (lm *TestLeaseManager) GetShardCount(ctx context.Context) (int, error) {
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

	return shardCount, nil
}

func (lm *TestLeaseManager) GetWorkerCount(ctx context.Context) (int, error) {
	if workerCountEnv := os.Getenv("KDS_WORKER_COUNT"); workerCountEnv != "" {
		count, err := strconv.Atoi(workerCountEnv)
		if err == nil && count > 0 {
			return count, nil
		}
	}

	if lm.k8sClient == nil {
		return 1, nil
	}

	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		return 1, nil
	}

	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespaceBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err == nil {
			namespace = string(namespaceBytes)
		} else {
			namespace = "default"
		}
	}

	pod, err := lm.k8sClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return 1, nil
	}

	if len(pod.OwnerReferences) == 0 {
		return 1, nil
	}

	for _, owner := range pod.OwnerReferences {
		switch owner.Kind {
		case "StatefulSet":
			statefulset, err := lm.k8sClient.AppsV1().StatefulSets(namespace).Get(ctx, owner.Name, metav1.GetOptions{})
			if err == nil && statefulset.Spec.Replicas != nil {
				return int(*statefulset.Spec.Replicas), nil
			}
		case "ReplicaSet":
			replicaset, err := lm.k8sClient.AppsV1().ReplicaSets(namespace).Get(ctx, owner.Name, metav1.GetOptions{})
			if err == nil && replicaset.Spec.Replicas != nil {
				return int(*replicaset.Spec.Replicas), nil
			}
		}
	}

	return 1, nil
}

func (lm *TestLeaseManager) CalculateMaxLeasesPerWorker(shardCount, workerCount int) int {
	if workerCount <= 0 {
		workerCount = 1
	}

	shardsPerWorker := int(math.Ceil(float64(shardCount) / float64(workerCount)))
	maxLeases := shardsPerWorker
	if maxLeases > MaxLeasePerWorkerLimit {
		maxLeases = MaxLeasePerWorkerLimit
	}

	return maxLeases
}

func (lm *TestLeaseManager) InitializeMetadataTable(ctx context.Context) error {
	_, err := lm.dynamodbClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(lm.metadataTable),
	})

	if err == nil {
		return nil
	}

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

	waitTimeout := 2 * time.Minute
	waitStart := time.Now()
	for {
		desc, err := lm.dynamodbClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(lm.metadataTable),
		})
		if err == nil && desc.Table != nil && desc.Table.TableStatus == types.TableStatusActive {
			return nil
		}
		if time.Since(waitStart) > waitTimeout {
			return fmt.Errorf("timeout waiting for metadata table to be active")
		}
		time.Sleep(2 * time.Second)
	}
}

func (lm *TestLeaseManager) SaveMetadata(ctx context.Context, metadata *LeaseMetadata) error {
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

	return err
}

func (lm *TestLeaseManager) GetMetadata(ctx context.Context) (*LeaseMetadata, error) {
	result, err := lm.dynamodbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(lm.metadataTable),
		Key: map[string]types.AttributeValue{
			"worker_id": &types.AttributeValueMemberS{Value: lm.workerID},
		},
		ConsistentRead: aws.Bool(true),
	})

	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, nil
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

func (lm *TestLeaseManager) getCoordinatorKey() string {
	return lm.appName + "_coordinator"
}

func (lm *TestLeaseManager) GetCoordinatorMetadata(ctx context.Context) (*LeaseMetadata, error) {
	coordinatorKey := lm.getCoordinatorKey()
	result, err := lm.dynamodbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(lm.metadataTable),
		Key: map[string]types.AttributeValue{
			"worker_id": &types.AttributeValueMemberS{Value: coordinatorKey},
		},
		ConsistentRead: aws.Bool(true),
	})

	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, nil
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

func (lm *TestLeaseManager) UpdateCoordinatorMetadata(ctx context.Context, newMetadata *LeaseMetadata, expectedShardCount, expectedWorkerCount int) error {
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
		var condCheckErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckErr) {
			return nil
		}
		return err
	}

	return nil
}

func (lm *TestLeaseManager) TryCreateCoordinatorMetadata(ctx context.Context, metadata *LeaseMetadata) (bool, error) {
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

	_, err := lm.dynamodbClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(lm.metadataTable),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(worker_id)"),
	})

	if err != nil {
		var condCheckErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckErr) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (lm *TestLeaseManager) InitializeMaxLeasesPerWorker(ctx context.Context) (int, error) {
	if err := lm.InitializeMetadataTable(ctx); err != nil {
		return 0, fmt.Errorf("failed to initialize metadata table: %w", err)
	}

	currentShardCount, err := lm.GetShardCount(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get shard count: %w", err)
	}

	currentWorkerCount, err := lm.GetWorkerCount(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get worker count: %w", err)
	}

	coordinatorMetadata, err := lm.GetCoordinatorMetadata(ctx)
	if err == nil && coordinatorMetadata != nil {
		configChanged := coordinatorMetadata.ShardCount != currentShardCount ||
			coordinatorMetadata.WorkerCount != currentWorkerCount

		if configChanged {
			newMaxLeasesPerWorker := lm.CalculateMaxLeasesPerWorker(currentShardCount, currentWorkerCount)

			updatedMetadata := &LeaseMetadata{
				WorkerID:           lm.getCoordinatorKey(),
				MaxLeasesPerWorker: newMaxLeasesPerWorker,
				StreamName:         lm.streamName,
				AppName:            lm.appName,
				ShardCount:         currentShardCount,
				WorkerCount:        currentWorkerCount,
			}

			err = lm.UpdateCoordinatorMetadata(ctx, updatedMetadata, coordinatorMetadata.ShardCount, coordinatorMetadata.WorkerCount)
			if err != nil {
				coordinatorMetadata, err = lm.GetCoordinatorMetadata(ctx)
				if err != nil {
					return 0, fmt.Errorf("failed to get updated coordinator metadata: %w", err)
				}
			} else {
				coordinatorMetadata = updatedMetadata
			}
		}

		workerMetadata := &LeaseMetadata{
			WorkerID:           lm.workerID,
			MaxLeasesPerWorker: coordinatorMetadata.MaxLeasesPerWorker,
			StreamName:         lm.streamName,
			AppName:            lm.appName,
			ShardCount:         coordinatorMetadata.ShardCount,
			WorkerCount:        coordinatorMetadata.WorkerCount,
		}
		lm.SaveMetadata(ctx, workerMetadata)

		return coordinatorMetadata.MaxLeasesPerWorker, nil
	}

	maxLeasesPerWorker := lm.CalculateMaxLeasesPerWorker(currentShardCount, currentWorkerCount)

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
		coordinatorMetadata, err = lm.GetCoordinatorMetadata(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to get coordinator metadata after creation attempt: %w", err)
		}
		if coordinatorMetadata == nil {
			return 0, fmt.Errorf("coordinator metadata not found after creation attempt")
		}
		maxLeasesPerWorker = coordinatorMetadata.MaxLeasesPerWorker
	}

	workerMetadata := &LeaseMetadata{
		WorkerID:           lm.workerID,
		MaxLeasesPerWorker: maxLeasesPerWorker,
		StreamName:         lm.streamName,
		AppName:            lm.appName,
		ShardCount:         currentShardCount,
		WorkerCount:        currentWorkerCount,
	}
	lm.SaveMetadata(ctx, workerMetadata)

	return maxLeasesPerWorker, nil
}
