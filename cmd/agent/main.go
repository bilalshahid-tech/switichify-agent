package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bilal/switchify-agent/internal/communicator"
	"github.com/bilal/switchify-agent/internal/config"
	"github.com/bilal/switchify-agent/internal/health"
	"github.com/bilal/switchify-agent/internal/logger"
	"github.com/bilal/switchify-agent/internal/monitor"

	"github.com/rs/zerolog/log"
)

func main() {

	// Load config
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	// Init logger
	logger.Init(cfg.Logging)
	log.Info().Str("agent", cfg.Agent.Name).Msg("starting switchify agent")

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// OS Signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	//------------------------------------------
	// START HEALTH SERVER
	//------------------------------------------
	healthSrv := health.New("8085")
	healthSrv.SetRunning(true)

	go func() {
		if err := healthSrv.Serve(); err != nil {
			log.Error().Err(err).Msg("health server stopped")
		}
	}()
	log.Info().Msg("health endpoint running on 127.0.0.1:8085/health")

	//------------------------------------------
	// START COMMUNICATOR
	//------------------------------------------
	comm := communicator.New(cfg)
	comm.Start()

	//------------------------------------------
	// START MONITOR + DECISION ENGINE
	//------------------------------------------
	mon := monitor.New(cfg, comm)
	go mon.Run(ctx)

	//------------------------------------------
	// WAIT FOR SHUTDOWN SIGNAL
	//------------------------------------------
	sig := <-sigChan
	log.Warn().Str("signal", sig.String()).Msg("shutdown signal received")

	//------------------------------------------
	// SHUTDOWN SEQUENCE
	//------------------------------------------
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	log.Info().Msg("stopping monitor...")
	cancel()
	<-shutdownCtx.Done()

	log.Info().Msg("stopping communicator...")
	comm.Shutdown(shutdownCtx)

	healthSrv.SetRunning(false)

	log.Info().Msg("agent stopped cleanly")
}
