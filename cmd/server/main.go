package main

import (
	"chat-libp2p/internal/server"
	"chat-libp2p/pkg/logger"
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {

	conf := server.NewConfig()
	if err := conf.Load(); err != nil {
		logger.Fatal("failed to load config: %v", err)
	}

	server := server.NewServer(conf)

	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2)
		for sig := range signals {
			switch sig {
			case syscall.SIGUSR1: // 10
				server.Dump()
			case syscall.SIGUSR2: // 12
			default:
				server.Close()
				return
			}
		}
	}()

	if err := server.Run(context.Background()); err != nil {
		logger.Fatal("failed to run server: %v", err)
	}
}
