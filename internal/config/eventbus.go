package config

import "time"

type EventBusConfig struct {
	// StreamName is the Redis stream used for durable event handoff.
	StreamName string `env:"DINCHY_EVENT_BUS_STREAM_NAME" validate:"required"`
	// ConsumerGroupPrefix is the prefix used to derive one consumer group per subscriber.
	ConsumerGroupPrefix string `env:"DINCHY_EVENT_BUS_CONSUMER_GROUP_PREFIX" validate:"required"`
	// ConsumerName identifies this process in each subscriber consumer group.
	ConsumerName string `env:"DINCHY_EVENT_BUS_CONSUMER_NAME"`
	// BatchSize is the maximum number of stream messages processed per subscriber pass.
	BatchSize int `env:"DINCHY_EVENT_BUS_BATCH_SIZE" validate:"gt=0"`
	// RetentionWindow controls how long messages remain eligible for replay in Redis.
	RetentionWindow time.Duration `env:"DINCHY_EVENT_BUS_RETENTION_WINDOW" validate:"gt=0"`
	// ClaimMinIdle controls when pending messages can be reclaimed after a crash.
	ClaimMinIdle time.Duration `env:"DINCHY_EVENT_BUS_CLAIM_MIN_IDLE" validate:"gt=0"`
	// ReadBlock controls how long consumers wait for new stream messages.
	ReadBlock time.Duration `env:"DINCHY_EVENT_BUS_READ_BLOCK" validate:"gt=0"`
	// WorkerInterval controls how often the subscriber worker wakes up.
	WorkerInterval time.Duration `env:"DINCHY_EVENT_BUS_WORKER_INTERVAL" validate:"gt=0"`
}

func DefaultEventBus() EventBusConfig {
	return EventBusConfig{
		StreamName:          "app.events",
		ConsumerGroupPrefix: "app",
		ConsumerName:        "local",
		BatchSize:           100,
		RetentionWindow:     5 * time.Minute,
		ClaimMinIdle:        2 * time.Minute,
		ReadBlock:           500 * time.Millisecond,
		WorkerInterval:      5 * time.Second,
	}
}
