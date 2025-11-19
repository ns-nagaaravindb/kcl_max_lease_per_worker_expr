package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
)

// Simple wrapper types to match the lease manager interfaces
type kinesisClient struct {
	*kinesis.Client
}

type dynamodbClient struct {
	*dynamodb.Client
}

var (
	isHealthy atomic.Bool
	isReady   atomic.Bool
)

func init() {
	isHealthy.Store(true)
	isReady.Store(false)
}

func main() {
	log.Println("Starting KDS Consumer Test Application...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get configuration from environment
	region := getEnv("AWS_REGION", "us-east-1")
	streamName := getEnv("STREAM_NAME", "test-stream")
	appName := getEnv("APP_NAME", "kds-consumer-app")
	workerID := getEnv("HOSTNAME", "worker-unknown")
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	enableDynamic := getEnv("ENABLE_DYNAMIC_MAX_LEASES", "true") == "true"

	log.Printf("Configuration: region=%s, stream=%s, app=%s, worker=%s, endpoint=%s, dynamic=%v",
		region, streamName, appName, workerID, endpoint, enableDynamic)

	// Start health check server
	go startHealthServer()

	// Give LocalStack time to be ready
	log.Println("Waiting for services to be ready...")
	time.Sleep(5 * time.Second)

	// Initialize AWS clients
	awsCfg, err := loadAWSConfig(ctx, region, endpoint)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	kinesisClient := kinesis.NewFromConfig(awsCfg)
	dynamodbClient := dynamodb.NewFromConfig(awsCfg)

	// Test AWS connectivity
	if err := testAWSConnectivity(ctx, kinesisClient, dynamodbClient, streamName); err != nil {
		log.Printf("WARNING: AWS connectivity test failed: %v", err)
		log.Println("Will retry in consumer loop...")
	}

	if !enableDynamic {
		log.Println("Dynamic max leases disabled, running in basic mode")
		isReady.Store(true)
		runBasicConsumer(ctx, kinesisClient, streamName, workerID)
		return
	}

	// Initialize lease manager (similar to the actual consumer code)
	log.Println("Initializing KDS Lease Manager...")
	leaseManager, err := NewTestLeaseManager(ctx, region, streamName, appName, workerID, endpoint)
	if err != nil {
		log.Fatalf("Failed to create lease manager: %v", err)
	}

	// Initialize max leases per worker
	maxLeases, err := leaseManager.InitializeMaxLeasesPerWorker(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize max leases per worker: %v", err)
	}

	log.Printf("✅ Successfully initialized! Max leases per worker: %d", maxLeases)
	isReady.Store(true)

	// Simulate consumer running
	log.Println("Consumer is now running and processing records...")
	log.Printf("Worker %s will acquire up to %d leases", workerID, maxLeases)

	// Periodic status updates
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			// Log periodic status
			metadata, err := leaseManager.GetMetadata(ctx)
			if err != nil {
				log.Printf("Failed to get metadata: %v", err)
			} else if metadata != nil {
				log.Printf("Status: worker=%s, maxLeases=%d, shards=%d, workers=%d",
					metadata.WorkerID, metadata.MaxLeasesPerWorker,
					metadata.ShardCount, metadata.WorkerCount)
			}

			// Check if configuration changed
			coordMetadata, err := leaseManager.GetCoordinatorMetadata(ctx)
			if err != nil {
				log.Printf("Failed to get coordinator metadata: %v", err)
			} else if coordMetadata != nil {
				if coordMetadata.MaxLeasesPerWorker != maxLeases {
					log.Printf("⚠️  Configuration changed detected! Old: %d, New: %d",
						maxLeases, coordMetadata.MaxLeasesPerWorker)
					log.Println("In real scenario, this would trigger reconfiguration")
				}
			}

		case sig := <-sigChan:
			log.Printf("Received signal %s, shutting down gracefully...", sig)
			isReady.Store(false)
			isHealthy.Store(false)
			time.Sleep(2 * time.Second) // Grace period
			return

		case <-ctx.Done():
			return
		}
	}
}

func loadAWSConfig(ctx context.Context, region, endpoint string) (aws.Config, error) {
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

	return config.LoadDefaultConfig(ctx, opts...)
}

func testAWSConnectivity(ctx context.Context, kc *kinesis.Client, dc *dynamodb.Client, streamName string) error {
	log.Println("Testing AWS connectivity...")

	// Test Kinesis
	_, err := kc.DescribeStream(ctx, &kinesis.DescribeStreamInput{
		StreamName: aws.String(streamName),
	})
	if err != nil {
		return fmt.Errorf("kinesis test failed: %w", err)
	}
	log.Println("✅ Kinesis connectivity OK")

	// Test DynamoDB
	_, err = dc.ListTables(ctx, &dynamodb.ListTablesInput{})
	if err != nil {
		return fmt.Errorf("dynamodb test failed: %w", err)
	}
	log.Println("✅ DynamoDB connectivity OK")

	return nil
}

func runBasicConsumer(ctx context.Context, kc *kinesis.Client, streamName, workerID string) {
	log.Println("Running in basic consumer mode (no dynamic lease management)")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Printf("Worker %s is processing records...", workerID)
		case <-ctx.Done():
			return
		}
	}
}

func startHealthServer() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if isHealthy.Load() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "OK")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "Unhealthy")
		}
	})

	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if isReady.Load() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Ready")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "Not Ready")
		}
	})

	log.Println("Health check server listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Health server failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
