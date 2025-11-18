package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/sirupsen/logrus"
	"github.com/vmware/vmware-go-kcl/clientlibrary/config"
	"github.com/vmware/vmware-go-kcl/clientlibrary/interfaces"
	"github.com/vmware/vmware-go-kcl/clientlibrary/worker"
	"gopkg.in/yaml.v3"
)

// Config represents the enhanced consumer configuration
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
	Consumer struct {
		ApplicationName                          string `yaml:"application_name"`
		WorkerID                                 string `yaml:"worker_id"`
		MaxRecords                               int    `yaml:"max_records"`
		CallProcessRecordsEvenForEmptyRecordList bool   `yaml:"call_process_records_even_for_empty_list"`

		// Advanced KCL settings for lease stealing and rebalancing
		EnableLeaseStealing          bool `yaml:"enable_lease_stealing"`
		MaxLeasesForWorker           int  `yaml:"max_leases_for_worker"`
		MaxLeasesToStealAtOneTime    int  `yaml:"max_leases_to_steal_at_one_time"`
		ShardSyncIntervalMillis      int  `yaml:"shard_sync_interval_millis"`
		LeaseStealingIntervalMillis  int  `yaml:"lease_stealing_interval_millis"`
		FailoverTimeMillis           int  `yaml:"failover_time_millis"`
		LeaseRefreshWaitTimeMillis   int  `yaml:"lease_refresh_wait_time_millis"`
		IdleTimeBetweenReadsInMillis int  `yaml:"idle_time_between_reads_in_millis"`

		// Checkpointing configuration
		CheckpointFrequencyCount  int `yaml:"checkpoint_frequency_count"`
		CheckpointFrequencyMillis int `yaml:"checkpoint_frequency_millis"`

		// Parent/Child shard processing
		ProcessParentShardBeforeChildren bool `yaml:"process_parent_shard_before_children"`

		// Number of pods for calculating max leases
		TotalNumPods int `yaml:"total_num_pods"`
	} `yaml:"consumer"`
}

// Event represents a sample data event
type Event struct {
	EventID   string                 `json:"event_id"`
	UserID    string                 `json:"user_id"`
	Timestamp time.Time              `json:"timestamp"`
	Action    string                 `json:"action"`
	Value     float64                `json:"value"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// EnhancedRecordProcessor implements the KCL RecordProcessor interface with enhanced features
type EnhancedRecordProcessor struct {
	shardID        string
	recordCount    int
	startTime      time.Time
	isParentShard  bool
	childShardIDs  []string
	processingRate float64
}

// Initialize is called once when the processor starts processing a shard
func (rp *EnhancedRecordProcessor) Initialize(input *interfaces.InitializationInput) {
	rp.shardID = input.ShardId
	rp.recordCount = 0
	rp.startTime = time.Now()

	log.Printf("[%s] 🚀 Initializing record processor", rp.shardID)
	log.Printf("[%s] ExtendedSequenceNumber: %v", rp.shardID, input.ExtendedSequenceNumber)
}

// ProcessRecords is called to process a batch of records from the shard
func (rp *EnhancedRecordProcessor) ProcessRecords(input *interfaces.ProcessRecordsInput) {
	batchStart := time.Now()

	// Process each record
	for _, record := range input.Records {
		var event Event
		if err := json.Unmarshal(record.Data, &event); err != nil {
			log.Printf("[%s] ❌ Failed to unmarshal record: %v", rp.shardID, err)
			continue
		}

		rp.recordCount++

		// Log every 10th record to reduce noise
		if rp.recordCount%10 == 0 {
			elapsed := time.Since(rp.startTime).Seconds()
			rate := float64(rp.recordCount) / elapsed
			rp.processingRate = rate

			log.Printf("[%s] 📊 Record #%d | Rate: %.2f rec/s | EventID: %s | UserID: %s | Action: %s",
				rp.shardID, rp.recordCount, rate, event.EventID, event.UserID, event.Action)
		}
	}

	// Checkpoint after processing records
	if len(input.Records) > 0 {
		lastRecord := input.Records[len(input.Records)-1]
		if err := input.Checkpointer.Checkpoint(lastRecord.SequenceNumber); err != nil {
			log.Printf("[%s] ❌ Failed to checkpoint: %v", rp.shardID, err)
		} else {
			batchDuration := time.Since(batchStart).Milliseconds()
			log.Printf("[%s] ✅ Checkpointed batch of %d records (took %dms)",
				rp.shardID, len(input.Records), batchDuration)
		}
	}
}

// Shutdown is called when the processor is shutting down
func (rp *EnhancedRecordProcessor) Shutdown(input *interfaces.ShutdownInput) {
	elapsed := time.Since(rp.startTime).Seconds()
	avgRate := float64(rp.recordCount) / elapsed

	log.Printf("[%s] 🛑 Shutting down. Reason: %v", rp.shardID, input.ShutdownReason)
	log.Printf("[%s] 📈 Statistics: %d records, %.2f seconds, %.2f rec/s",
		rp.shardID, rp.recordCount, elapsed, avgRate)

	// Checkpoint on graceful shutdown (TERMINATE or ZOMBIE)
	switch input.ShutdownReason {
	case interfaces.TERMINATE:
		// Shard has been closed (split or merged)
		log.Printf("[%s] 🔄 Shard TERMINATED (likely split/merged). Child shards can now be processed.", rp.shardID)
		if err := input.Checkpointer.Checkpoint(nil); err != nil {
			log.Printf("[%s] ❌ Failed to checkpoint on TERMINATE: %v", rp.shardID, err)
		}
	case interfaces.ZOMBIE:
		// This worker lost the lease to another worker
		log.Printf("[%s] 👻 Shard became ZOMBIE (lease stolen by another worker)", rp.shardID)
		// Don't checkpoint on ZOMBIE - let the new owner continue from last checkpoint
	case interfaces.REQUESTED:
		// Explicit shutdown requested (e.g., application termination)
		log.Printf("[%s] 🔌 Shutdown REQUESTED (application terminating)", rp.shardID)
		// DON'T checkpoint on REQUESTED!
		// Checkpointing with nil marks the shard as SHARD_END, preventing restart.
		// The shard is still OPEN in Kinesis, so we should let it resume from the last checkpoint.
		log.Printf("[%s] ℹ️  Not checkpointing - shard will resume from last position on restart", rp.shardID)
	}
}

// EnhancedRecordProcessorFactory creates new EnhancedRecordProcessor instances
type EnhancedRecordProcessorFactory struct{}

// CreateProcessor creates a new EnhancedRecordProcessor for a shard
func (f *EnhancedRecordProcessorFactory) CreateProcessor() interfaces.IRecordProcessor {
	return &EnhancedRecordProcessor{}
}

func loadConfig() (*Config, error) {
	// Check for custom config file path from environment variable
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/config-pod1.yaml"
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configFile, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	log.Printf("✅ Loaded configuration from: %s", configFile)
	return &cfg, nil
}

// calculateMaxLeasesForWorker implements: MaxLeasesForWorker = min(10, NumShards/NumPODs)
// Note: We can't know NumShards at config time, so we set a high value and rely on
// the KCL library's internal lease stealing logic
func calculateMaxLeasesForWorker(totalShards, totalPods int) int {
	if totalPods == 0 {
		return 10
	}

	calculatedMax := totalShards / totalPods

	// Apply the min(10, NumShards/NumPODs) formula
	if calculatedMax > 10 {
		return 10
	}

	return calculatedMax
}

func main() {
	log.Println("=" + "=")
	log.Println("🚀 Starting Enhanced KCL Consumer with Lease Stealing")
	log.Println("=" + "=")

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	log.Printf("📝 Worker ID: %s", cfg.Consumer.WorkerID)
	log.Printf("📝 Application: %s", cfg.Consumer.ApplicationName)
	log.Printf("📝 Stream: %s", cfg.Kinesis.StreamName)
	log.Printf("📝 Lease Stealing: %v", cfg.Consumer.EnableLeaseStealing)
	log.Printf("📝 Max Leases For Worker: %d", cfg.Consumer.MaxLeasesForWorker)
	log.Printf("📝 Process Parent Before Children: %v", cfg.Consumer.ProcessParentShardBeforeChildren)

	// Enable detailed logging for KCL library
	logrus.SetLevel(logrus.InfoLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	// Configure KCL
	kclConfig := config.NewKinesisClientLibConfig(
		cfg.Consumer.ApplicationName,
		cfg.Kinesis.StreamName,
		cfg.AWS.Region,
		cfg.Consumer.WorkerID,
	)

	// Set LocalStack endpoints
	kclConfig.KinesisEndpoint = cfg.AWS.Endpoint
	kclConfig.DynamoDBEndpoint = cfg.AWS.Endpoint

	// Set credentials for LocalStack
	kclConfig.KinesisCredentials = credentials.NewStaticCredentials(cfg.AWS.AccessKey, cfg.AWS.SecretKey, "")
	kclConfig.DynamoDBCredentials = credentials.NewStaticCredentials(cfg.AWS.AccessKey, cfg.AWS.SecretKey, "")

	// Set processing configuration
	kclConfig.InitialPositionInStream = config.TRIM_HORIZON // Read from beginning of stream
	kclConfig.MaxRecords = cfg.Consumer.MaxRecords
	kclConfig.CallProcessRecordsEvenForEmptyRecordList = cfg.Consumer.CallProcessRecordsEvenForEmptyRecordList

	// ===== CRITICAL: Lease Stealing Configuration =====
	// NOTE: The vmware-go-kcl library may have limitations in lease stealing support.
	// These settings are configured based on standard KCL behavior:

	// Set max leases per worker
	if cfg.Consumer.MaxLeasesForWorker > 0 {
		kclConfig.MaxLeasesForWorker = cfg.Consumer.MaxLeasesForWorker
		log.Printf("🎯 MaxLeasesForWorker set to: %d", cfg.Consumer.MaxLeasesForWorker)
	}

	// Set max leases to steal at one time (conservative approach)
	if cfg.Consumer.MaxLeasesToStealAtOneTime > 0 {
		kclConfig.MaxLeasesToStealAtOneTime = cfg.Consumer.MaxLeasesToStealAtOneTime
		log.Printf("🎯 MaxLeasesToStealAtOneTime set to: %d", cfg.Consumer.MaxLeasesToStealAtOneTime)
	}

	// Set shard sync interval (how often to check for new shards)
	if cfg.Consumer.ShardSyncIntervalMillis > 0 {
		kclConfig.ShardSyncIntervalMillis = cfg.Consumer.ShardSyncIntervalMillis
		log.Printf("🔄 ShardSyncIntervalMillis set to: %d", cfg.Consumer.ShardSyncIntervalMillis)
	}

	// Set failover time (time before lease is considered expired)
	if cfg.Consumer.FailoverTimeMillis > 0 {
		kclConfig.FailoverTimeMillis = cfg.Consumer.FailoverTimeMillis
		log.Printf("⏱️  FailoverTimeMillis set to: %d", cfg.Consumer.FailoverTimeMillis)
	}

	// Set idle time between reads
	if cfg.Consumer.IdleTimeBetweenReadsInMillis > 0 {
		kclConfig.IdleTimeBetweenReadsInMillis = cfg.Consumer.IdleTimeBetweenReadsInMillis
		log.Printf("💤 IdleTimeBetweenReadsInMillis set to: %d", cfg.Consumer.IdleTimeBetweenReadsInMillis)
	}

	// ===== Parent/Child Shard Processing Configuration =====
	// Setting this to false allows child shards to be processed immediately
	// without waiting for parent shards to complete
	// Note: This is not directly supported by vmware-go-kcl.  need to use customized library
	log.Printf("👪 ProcessParentShardBeforeChildren: %v", cfg.Consumer.ProcessParentShardBeforeChildren)
	if !cfg.Consumer.ProcessParentShardBeforeChildren {
		log.Println("⚠️  Child shards will start processing immediately (if supported by library)")
	}

	// Create worker with enhanced record processor
	recordProcessorFactory := &EnhancedRecordProcessorFactory{}
	kclWorker := worker.NewWorker(recordProcessorFactory, kclConfig)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start the worker in a goroutine
	log.Println("=" + "=")
	log.Println("✅ Consumer is running. Press Ctrl+C to stop.")
	log.Println("=" + "=")

	errChan := make(chan error, 1)
	go func() {
		if err := kclWorker.Start(); err != nil {
			errChan <- err
		}
	}()

	// Wait for either shutdown signal or error
	select {
	case <-sigChan:
		log.Println("🛑 Received shutdown signal...")
		kclWorker.Shutdown()
	case err := <-errChan:
		log.Fatalf("❌ Worker failed: %v", err)
	}

	log.Println("=" + "=")
	log.Println("✅ Consumer stopped gracefully.")
	log.Println("=" + "=")
}
