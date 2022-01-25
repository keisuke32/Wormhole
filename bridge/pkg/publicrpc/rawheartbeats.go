package publicrpc

import (
	gossipv1 "github.com/certusone/wormhole/bridge/pkg/proto/gossip/v1"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"math/rand"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// track the number of active connections
var (
	currentPublicHeartbeatStreamsOpen = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wormhole_publicrpc_rawheartbeat_connections",
			Help: "Current number of clients consuming gRPC raw heartbeat streams",
		})
)

// RawHeartbeatConns holds the multiplexing state required for distribution of
// heartbeat messages to all the open connections.
type RawHeartbeatConns struct {
	mu     sync.RWMutex
	subs   map[int]chan<- *gossipv1.Heartbeat
	logger *zap.Logger
}

func HeartbeatStreamMultiplexer(logger *zap.Logger) *RawHeartbeatConns {
	ps := &RawHeartbeatConns{
		subs:   map[int]chan<- *gossipv1.Heartbeat{},
		logger: logger.Named("heartbeatmultiplexer"),
	}
	return ps
}

// getUniqueClientId loops to generate & test integers for existence as key of map. returns an int that is not a key in map.
func (ps *RawHeartbeatConns) getUniqueClientId() int {
	clientId := rand.Intn(1e6)
	found := false
	for found {
		clientId = rand.Intn(1e6)
		_, found = ps.subs[clientId]
	}
	return clientId
}

// subscribeHeartbeats adds a channel to the subscriber map, keyed by arbitrary clientId
func (ps *RawHeartbeatConns) subscribeHeartbeats(ch chan *gossipv1.Heartbeat) int {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	clientId := ps.getUniqueClientId()
	ps.logger.Info("subscribeHeartbeats for client", zap.Int("client", clientId))
	ps.subs[clientId] = ch
	currentPublicHeartbeatStreamsOpen.Set(float64(len(ps.subs)))
	return clientId
}

// PublishHeartbeat sends a message to all channels in the subscription map
func (ps *RawHeartbeatConns) PublishHeartbeat(msg *gossipv1.Heartbeat) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	for client, ch := range ps.subs {
		select {
		case ch <- msg:
			ps.logger.Debug("published message to client", zap.Int("client", client))
		default:
			ps.logger.Debug("buffer overrun when attempting to publish message", zap.Int("client", client))
		}
	}
}

// unsubscribeHeartbeats removes the client's channel from the subscription map
func (ps *RawHeartbeatConns) unsubscribeHeartbeats(clientId int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.logger.Debug("unsubscribeHeartbeats for client", zap.Int("clientId", clientId))
	delete(ps.subs, clientId)
	currentPublicHeartbeatStreamsOpen.Set(float64(len(ps.subs)))
}
