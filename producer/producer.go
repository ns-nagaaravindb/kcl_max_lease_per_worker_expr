package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	AWS struct {
		Region    string `yaml:"region"`
		Endpoint  string `yaml:"endpoint"`
		AccessKey string `yaml:"access_key"`
		SecretKey string `yaml:"secret_key"`
	} `yaml:"aws"`
	Kinesis struct {
		StreamName string `yaml:"stream_name"`
	} `yaml:"kinesis"`
	Producer struct {
		BatchSize     int `yaml:"batch_size"`
		BatchDelayMs  int `yaml:"batch_delay_ms"`
		TotalMessages int `yaml:"total_messages"`
		NumShards     int `yaml:"num_shards"`
	} `yaml:"producer"`
}

// Event represents a sample data event
type Event struct {
	EventID   string                 `json:"event_id"`
	UserID    string                 `json:"user_id"`
	Timestamp time.Time              `json:"timestamp"`
	Action    string                 `json:"action"`
	Value     float64                `json:"value"`
	Metadata  map[string]interface{} `json:"metadata"`
	ShardKey  string                 `json:"shard_key"`
}

var actions = []string{"login", "purchase", "view", "click", "logout", "search", "add_to_cart", "checkout"}

func loadConfig() (*Config, error) {
	configFile := "../config/config-20-shards.yaml"
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

func generateEvent(numShards int) *Event {
	// Generate a deterministic shard key to evenly distribute across shards
	shardKey := fmt.Sprintf("shard-key-%d", rand.Intn(numShards))

	return &Event{
		EventID:   fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		UserID:    fmt.Sprintf("user_%d", rand.Intn(10000)),
		Timestamp: time.Now(),
		Action:    actions[rand.Intn(len(actions))],
		Value:     rand.Float64() * 1000,
		Metadata: map[string]interface{}{
			"source":     "producer",
			"version":    "2.0",
			"session":    fmt.Sprintf("sess_%d", rand.Intn(1000)),
			"experiment": "mohan-20-shards",
		},
		ShardKey: shardKey,
	}
}

func main() {
	log.Println("========================================")
	log.Println("🚀 Starting Kinesis Producer (20 Shards)")
	log.Println("========================================")

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	// Initialize AWS Config
	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.AWS.Region),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:               cfg.AWS.Endpoint,
					HostnameImmutable: true,
				}, nil
			})),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AWS.AccessKey,
			cfg.AWS.SecretKey,
			"",
		)),
	)
	if err != nil {
		log.Fatalf("❌ Failed to load AWS config: %v", err)
	}

	// Create Kinesis client
	client := kinesis.NewFromConfig(awsCfg)

	log.Printf("📝 Stream: %s", cfg.Kinesis.StreamName)
	log.Printf("📝 Configuration: BatchSize=%d, BatchDelay=%dms, TotalMessages=%d, NumShards=%d",
		cfg.Producer.BatchSize, cfg.Producer.BatchDelayMs, cfg.Producer.TotalMessages, cfg.Producer.NumShards)

	// Verify stream exists and has correct shard count
	describeOutput, err := client.DescribeStream(ctx, &kinesis.DescribeStreamInput{
		StreamName: aws.String(cfg.Kinesis.StreamName),
	})
	if err != nil {
		log.Fatalf("❌ Failed to describe stream: %v", err)
	}

	actualShardCount := len(describeOutput.StreamDescription.Shards)
	log.Printf("✅ Stream has %d shards", actualShardCount)

	if actualShardCount != cfg.Producer.NumShards {
		log.Printf("⚠️  Warning: Expected %d shards but found %d", cfg.Producer.NumShards, actualShardCount)
	}

	messageCount := 0
	startTime := time.Now()
	shardDistribution := make(map[string]int)

	log.Println("========================================")
	log.Println("✅ Producer is running. Press Ctrl+C to stop.")
	log.Println("========================================")

	for {
		// Check if we've reached the total message limit
		if cfg.Producer.TotalMessages > 0 && messageCount >= cfg.Producer.TotalMessages {
			log.Printf("✅ Reached total message limit: %d messages", cfg.Producer.TotalMessages)
			break
		}

		// Send batch of messages
		for i := 0; i < cfg.Producer.BatchSize; i++ {
			event := generateEvent(cfg.Producer.NumShards)
			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("❌ Failed to marshal event: %v", err)
				continue
			}

			// Use the shard key for consistent distribution
			input := &kinesis.PutRecordInput{
				StreamName:   aws.String(cfg.Kinesis.StreamName),
				Data:         data,
				PartitionKey: aws.String(event.ShardKey),
			}

			output, err := client.PutRecord(ctx, input)
			if err != nil {
				log.Printf("❌ Failed to put record: %v", err)
				continue
			}

			messageCount++
			shardDistribution[*output.ShardId]++

			// Log every 100th message
			if messageCount%100 == 0 {
				log.Printf("[%d] 📤 EventID: %s | UserID: %s | Action: %s | ShardID: %s",
					messageCount, event.EventID, event.UserID, event.Action, *output.ShardId)
			}

			// Break if we've reached the limit mid-batch
			if cfg.Producer.TotalMessages > 0 && messageCount >= cfg.Producer.TotalMessages {
				break
			}
		}

		// Calculate and display stats every batch
		elapsed := time.Since(startTime).Seconds()
		rate := float64(messageCount) / elapsed

		// Count unique shards that have received messages
		uniqueShards := len(shardDistribution)

		log.Printf("📊 Stats: Total=%d, Rate=%.2f msgs/sec, Elapsed=%.2fs, UniqueShards=%d/%d",
			messageCount, rate, elapsed, uniqueShards, actualShardCount)

		// Wait before next batch
		if cfg.Producer.TotalMessages == 0 || messageCount < cfg.Producer.TotalMessages {
			time.Sleep(time.Duration(cfg.Producer.BatchDelayMs) * time.Millisecond)
		}
	}

	elapsed := time.Since(startTime).Seconds()
	uniqueShards := len(shardDistribution)

	log.Println("========================================")
	log.Printf("✅ Producer completed!")
	log.Printf("📊 Total Messages: %d", messageCount)
	log.Printf("📊 Duration: %.2f seconds", elapsed)
	log.Printf("📊 Rate: %.2f msgs/sec", float64(messageCount)/elapsed)
	log.Printf("📊 Unique Shards Used: %d/%d", uniqueShards, actualShardCount)
	log.Printf("📊 Average Messages per Shard: %.2f", float64(messageCount)/float64(uniqueShards))
	log.Println("========================================")
}
