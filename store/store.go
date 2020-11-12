package store

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

const (
	retainSnapshotCount = 2
	maxPool             = 3
	raftTimeout         = 10 * time.Second
)

// Store is a simple key-value store, where all changes are made via Raft consensus.
type Store struct {
	raft *raft.Raft
	mu   sync.Mutex
	m    map[string]interface{}
}

// New returns a new Store.
func New() *Store {
	return &Store{
		m: make(map[string]interface{}),
	}
}

// Open opens the store. If peers is empty then this node
// becomes the first node, and therefore leader, of the cluster.
func (s *Store) Open(nodeID string, stateDir string, kvStore string, raftBindAddrPort string, raftAdvertiseAddrPort string, peers string, logger hclog.Logger) error {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)

	var snapshots raft.SnapshotStore
	var logStore raft.LogStore
	var stableStore raft.StableStore

	switch kvStore {
	case "memory":
		snapshots = raft.NewInmemSnapshotStore()
		logStore = raft.NewInmemStore()
		stableStore = raft.NewInmemStore()
	case "boltdb":
		var err error
		err = os.MkdirAll(stateDir, 0700)
		if err != nil {
			return fmt.Errorf("couldn't create state directory: %w", err)
		}
		snapshots, err = raft.NewFileSnapshotStoreWithLogger(stateDir, retainSnapshotCount, logger)
		if err != nil {
			return fmt.Errorf("file snapshot store: %w", err)
		}
		boltDB, err := raftboltdb.NewBoltStore(filepath.Join(stateDir, "raft.db"))
		if err != nil {
			return fmt.Errorf("new bolt store: %s", err)
		}
		logStore = boltDB
		stableStore = boltDB
	default:
		return fmt.Errorf("unknown kvstore (only 'boltdb' and 'memory' are supported)")
	}

	raftAdvertiseTCPAddr, err := net.ResolveTCPAddr("tcp", raftAdvertiseAddrPort)
	if err != nil {
		return err
	}
	transport, err := raft.NewTCPTransportWithLogger(raftBindAddrPort, raftAdvertiseTCPAddr, maxPool, raftTimeout, logger)
	if err != nil {
		return err
	}

	raftInstance, err := raft.NewRaft(config, (*fsm)(s), logStore, stableStore, snapshots, transport)
	if err != nil {
		return fmt.Errorf("new raft: %w", err)
	}

	if peers == "" {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      config.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		raftInstance.BootstrapCluster(configuration)
	} else {
		individualPeers := strings.Split(peers, ",")
		peerServers := make([]raft.Server, len(individualPeers))
		for i, individualPeer := range individualPeers {
			individualPeerParts := strings.Split(individualPeer, "@")
			if len(individualPeerParts) != 2 {
				return fmt.Errorf("incorrect peer format (not node1@addr1:port1,node2@addr2:port2,...)")
			}
			peerServers[i] = raft.Server{
				ID:      raft.ServerID(individualPeerParts[0]),
				Address: raft.ServerAddress(individualPeerParts[1]),
			}
		}
		configuration := raft.Configuration{
			Servers: peerServers,
		}
		raftInstance.BootstrapCluster(configuration)
	}

	s.raft = raftInstance

	logger.Info("initialized raft", "raftInstance", raftInstance)
	return nil
}

// Get returns the value for the given key.
func (s *Store) Get(key string) (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[key], nil
}

// Set sets the value for the given key.
func (s *Store) Set(key string, value interface{}) error {
	if s.raft.State() != raft.Leader {
		return fmt.Errorf("not leader")
	}

	c := &command{
		Op:    "set",
		Key:   key,
		Value: value,
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}

	f := s.raft.Apply(b, raftTimeout)
	return f.Error()
}

// Delete deletes the given key.
func (s *Store) Delete(key string) error {
	if s.raft.State() != raft.Leader {
		return fmt.Errorf("not leader")
	}

	c := &command{
		Op:  "delete",
		Key: key,
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}

	f := s.raft.Apply(b, raftTimeout)
	return f.Error()
}

// ObservationCh retrieves the channel where cluster changes are reported
func (s *Store) ObservationCh() chan raft.Observation {
	observationCh := make(chan raft.Observation, 1024)
	s.raft.RegisterObserver(raft.NewObserver(observationCh, false, nil))
	return observationCh
}
