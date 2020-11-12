package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/adi/raftstarter/httpd"
	"github.com/adi/raftstarter/store"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
)

var logger hclog.Logger

func init() {
	logger = hclog.New(&hclog.LoggerOptions{
		Name:  "integrations",
		Level: hclog.Debug,
	})
}

func main() {

	// Read my hostname
	myHostname, err := os.Hostname()
	if err != nil {
		panic(fmt.Errorf("get hostname: %w", err))
	}

	// Read the IP of that hostname
	myIPs, err := net.DefaultResolver.LookupIP(context.Background(), "ip4", myHostname)
	if err != nil {
		log.Panicf("read own IP: %v", err)
	}
	myIP := myIPs[0].String()

	// Read config from environment
	nodeID := myHostname
	if envVar := os.Getenv("NODE_ID"); envVar != "" {
		nodeID = envVar
	}
	stateDir := fmt.Sprintf("/tmp/%s", nodeID)
	if envVar := os.Getenv("STATE_DIR"); envVar != "" {
		stateDir = envVar
	}
	kvStore := "boltdb"
	if envVar := os.Getenv("KV_STORE"); envVar != "" {
		kvStore = envVar
	}
	httpAddr := "0.0.0.0"
	if envVar := os.Getenv("HTTP_ADDR"); envVar != "" {
		httpAddr = envVar
	}
	httpPort := "11000"
	if envVar := os.Getenv("HTTP_PORT"); envVar != "" {
		httpPort = envVar
	}
	raftAddr := "0.0.0.0"
	if envVar := os.Getenv("RAFT_ADDR"); envVar != "" {
		raftAddr = envVar
	}
	raftPort := "12000"
	if envVar := os.Getenv("RAFT_PORT"); envVar != "" {
		raftPort = envVar
	}
	raftAdvertiseAddr := myIP
	if envVar := os.Getenv("RAFT_ADVERTISE_ADDR"); envVar != "" {
		raftAdvertiseAddr = envVar
	}
	raftAdvertisePort := raftPort
	if envVar := os.Getenv("RAFT_ADVERTISE_PORT"); envVar != "" {
		raftAdvertisePort = envVar
	}
	peers := fmt.Sprintf("%s@%s:%s", nodeID, raftAdvertiseAddr, raftAdvertisePort)
	if envVar := os.Getenv("PEERS"); envVar != "" {
		peers = envVar
	}
	httpAddrPort := fmt.Sprintf("%s:%s", httpAddr, httpPort)
	raftAddrPort := fmt.Sprintf("%s:%s", raftAddr, raftPort)
	raftAdvertiseAddrPort := fmt.Sprintf("%s:%s", raftAdvertiseAddr, raftAdvertisePort)
	logger.Info("start with config",
		"nodeID", nodeID,
		"stateDir", stateDir,
		"httpAddrPort", httpAddrPort,
		"raftAddrPort", raftAddrPort,
		"raftAdvertiseAddrPort", raftAdvertiseAddrPort,
		"peers", peers)

	// Create & open store
	s := store.New()
	err = s.Open(nodeID, stateDir, kvStore, raftAddrPort, raftAdvertiseAddrPort, peers, logger)
	if err != nil {
		panic(fmt.Errorf("open raft: %w", err))
	}

	// Create & serve httpd
	h := httpd.New(httpAddrPort, s, logger)
	if err := h.Start(); err != nil {
		log.Panicf("failed to start HTTP service: %v", err)
	}

	// Start observing raft state changes
	go func() {
		ch := s.ObservationCh()
		for {
			select {
			case o := <-ch:
				if raftState, ok := o.Data.(raft.RaftState); ok {
					logger.Info("new raft state", "raftState", raftState.String())
				}
			}
		}
	}()

	logger.Info("completed initialization")

	// React properly to signals
	sg := make(chan os.Signal)
	signal.Notify(sg, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for {
		select {
		case signal := <-sg:
			switch signal {
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("exit", "program", os.Args[0], "signal", signal)
				os.Exit(0)
			case syscall.SIGHUP:
				logger.Info("reload config", "program", os.Args[0], "signal", signal)
			}
		}
	}

}
