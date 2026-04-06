package communicator

import (
	"context"
	"sync"
	"github.com/bilal/switchify-agent/internal/config"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	maxBatchSize = 100
)

// ---------- Payload Types ----------

type MetricsPayload struct {
	Level      string `json:"level"`
	IspState   string `json:"isp_state"`
	LatencyMs  int    `json:"latency_ms"`
	PacketLoss int    `json:"packet_loss"`
	JitterMs   int    `json:"jitter_ms"`
	Time       string `json:"time"`
	Message    string `json:"message"`

	CorrelationID string `json:"-"`
}

type LogPayload struct {
	Level   string `json:"level"`
	Time    string `json:"time"`
	Message string `json:"message"`

	Error   string `json:"error,omitempty"`
	Attempt int    `json:"attempt,omitempty"`
	Count   int    `json:"count,omitempty"`

	CorrelationID string `json:"-"`
}

// ---------- Communicator ----------

type Communicator struct {
	cfg *config.Config

	metricsQueue chan MetricsPayload
	logsQueue    chan LogPayload

	producer *KafkaProducer

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ---------- Constructor ----------

func New(cfg *config.Config) *Communicator {
	queueSize := cfg.Agent.MaxQueueSize
	if queueSize <= 0 {
		queueSize = 1000
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Communicator{
		cfg: cfg,

		metricsQueue: make(chan MetricsPayload, queueSize),
		logsQueue:    make(chan LogPayload, queueSize),

		ctx:    ctx,
		cancel: cancel,
	}
}

// ---------- Lifecycle ----------

func (c *Communicator) Start() {
	producer, err :=NewKafkaProducer(c.cfg)
	if err !=nil{
		panic(err)
	}
	c.producer=producer
	c.wg.Add(2)
	go c.metricsLoop()
	go c.logsLoop()

	log.Info().
		Msg("Kafka communicator started")
}

func (c *Communicator) Shutdown(ctx context.Context) {
	c.cancel()

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("communicator shutdown complete")
	case <-ctx.Done():
		log.Warn().Msg("communicator shutdown timeout")
	}
	_=c.producer.metricsWriter.Close()
	_=c.producer.logsWriter.Close()
}

// ---------- Public API ----------

func (c *Communicator) SendMetrics(m MetricsPayload) {
	if m.CorrelationID == "" {
		m.CorrelationID = uuid.New().String()
	}

	select {
	case c.metricsQueue <- m:
	default:
		log.Warn().Msg("metrics dropped: queue full")
	}
}

func (c *Communicator) SendLog(l LogPayload) {
	if l.CorrelationID == "" {
		l.CorrelationID = uuid.New().String()
	}

	select {
	case c.logsQueue <- l:
	default:
		log.Warn().Msg("log dropped: queue full")
	}
}

// ---------- Loops ----------

func (c *Communicator) metricsLoop() {
	defer c.wg.Done()
	for{
		select
		{
		case<-c.ctx.Done():
		     return

		case m:=<-c.metricsQueue:
			if err := c.producer.PublishMetric(c.ctx,m); err != nil {
				log.Error().Err(err).Msg("failed to publish metric to kafka")
			}
	}
}
}

func (c *Communicator) logsLoop() {
	defer c.wg.Done()
	for{
		select{
		case<-c.ctx.Done():
	        return

		case l:=<-c.logsQueue:
			if err := c.producer.PublishLog(c.ctx, l); err != nil {
				log.Error().Err(err).Msg("failed to publish log to kafka")
			}
		}
	}
}


