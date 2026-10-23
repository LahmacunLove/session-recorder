package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	cspb "github.com/pascalhuerst/session-recorder/protocols/go/chunksink"
	cmpb "github.com/pascalhuerst/session-recorder/protocols/go/common"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// Callbacks that can be provided by the caller to react to events.
type OnRecorderStatusCB func(ctx context.Context, status *cmpb.RecorderStatus) error
type OnChunksCB func(ctx context.Context, chunks *cspb.Chunks) error
type OnRecorderConnectedCB func(recorderID uuid.UUID)
type OnRecorderDisconnectedCB func(recorderID uuid.UUID)

// ChunkSinkServerConfig contains all configuration and optional callbacks.
type ChunkSinkServerConfig struct {
	Name    string
	Version string

	OnRecorderStatusCB       OnRecorderStatusCB
	OnChunksCB               OnChunksCB
	OnRecorderConnectedCB    OnRecorderConnectedCB
	OnRecorderDisconnectedCB OnRecorderDisconnectedCB
}

// ChunkSinkServer implements the ChunkSink gRPC service and keeps track of
// active recorder connections so that commands (CutSession, etc.) can be routed.
type ChunkSinkServer struct {
	config *ChunkSinkServerConfig

	mu sync.Mutex
	// The command stream each recorder is reachable through: the newest one
	// it opened. A recorder can briefly have several (a reconnect while the
	// old stream has not noticed it is dead yet); only the stream registered
	// here may remove the entry again.
	commandStreams map[uuid.UUID]commandStream
	nextStreamID   uint64
}

type commandStream struct {
	id   uint64
	send func(*cspb.Command) error
}

// NewChunkSinkServer constructs a new server instance with the provided config.
func NewChunkSinkServer(config *ChunkSinkServerConfig) *ChunkSinkServer {
	return &ChunkSinkServer{
		config:         config,
		commandStreams: make(map[uuid.UUID]commandStream),
	}
}

// registerGrpcServer registers the service implementation with a gRPC server.
func (s *ChunkSinkServer) registerGrpcServer(server *grpc.Server) {
	cspb.RegisterChunkSinkServer(server, s)
}

// serverOptions: keepalive, so a command stream that died silently (an
// idle connection over Wi-Fi) is noticed in seconds rather than hours. The
// enforcement policy lets recorders ping as often as every 10 s, also with
// no call running - without it the server would answer their keepalive
// pings with GOAWAY "too_many_pings".
func (s *ChunkSinkServer) serverOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
	}
}

// announcement data is published via mDNS to advertise the service.
func (s *ChunkSinkServer) announcement() [][]byte {
	return [][]byte{
		[]byte(fmt.Sprintf("Chunk Sink Server: %s", s.config.Name)),
		[]byte(fmt.Sprintf("Software Version: %s", s.config.Version)),
	}
}

// SetRecorderStatus processes status updates from recorders.
func (s *ChunkSinkServer) SetRecorderStatus(ctx context.Context, in *cmpb.RecorderStatus) (*cmpb.Respone, error) {
	if s.config.OnRecorderStatusCB != nil {
		if err := s.config.OnRecorderStatusCB(ctx, in); err != nil {
			return &cmpb.Respone{Success: false, ErrorMessage: err.Error()}, err
		}
	}

	return &cmpb.Respone{Success: true}, nil
}

// SetChunks processes incoming audio chunks from recorders.
func (s *ChunkSinkServer) SetChunks(ctx context.Context, in *cspb.Chunks) (*cmpb.Respone, error) {
	if s.config.OnChunksCB != nil {
		if err := s.config.OnChunksCB(ctx, in); err != nil {
			return &cmpb.Respone{Success: false, ErrorMessage: err.Error()}, err
		}
	}

	return &cmpb.Respone{Success: true}, nil
}

// GetCommands establishes a command stream for a recorder. The stream's send
// function is stored so commands can be routed later (e.g., CutSession).
func (s *ChunkSinkServer) GetCommands(request *cspb.GetCommandRequest, server cspb.ChunkSink_GetCommandsServer) error {
	recorderID, err := uuid.Parse(request.GetRecorderID())
	if err != nil {
		log.Warn().Str("recorder-id", request.GetRecorderID()).Err(err).Msg("GetCommands: invalid recorder id")
		return fmt.Errorf("invalid recorder id %q: %w", request.GetRecorderID(), err)
	}

	s.mu.Lock()
	s.nextStreamID++
	streamID := s.nextStreamID
	s.commandStreams[recorderID] = commandStream{id: streamID, send: server.Send}
	mapSize := len(s.commandStreams)
	if s.config.OnRecorderConnectedCB != nil {
		go s.config.OnRecorderConnectedCB(recorderID)
	}
	s.mu.Unlock()

	log.Info().
		Stringer("recorder-id", recorderID).
		Int("connected-recorders", mapSize).
		Msg("GetCommands: stream opened")

	<-server.Context().Done()
	ctxErr := server.Context().Err()

	s.mu.Lock()
	// Only if it is still this stream: a stale stream closing after the
	// recorder has opened a newer one must not take the newer one with it.
	current, registered := s.commandStreams[recorderID]
	stillOurs := registered && current.id == streamID
	if stillOurs {
		delete(s.commandStreams, recorderID)
		if s.config.OnRecorderDisconnectedCB != nil {
			go s.config.OnRecorderDisconnectedCB(recorderID)
		}
	}
	mapSize = len(s.commandStreams)
	s.mu.Unlock()

	log.Info().
		Stringer("recorder-id", recorderID).
		Int("connected-recorders", mapSize).
		AnErr("ctx-err", ctxErr).
		Bool("superseded", !stillOurs).
		Msg("GetCommands: stream closed")

	return ctxErr
}

// CutSession forwards a cut command to the recorder if a command stream exists.
func (s *ChunkSinkServer) CutSession(recorderID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if stream, ok := s.commandStreams[recorderID]; ok {
		return stream.send(&cspb.Command{
			Command: &cspb.Command_CmdCutSession{
				CmdCutSession: &cspb.CmdCutSession{},
			},
		})
	}

	return fmt.Errorf("no connection to recorder %s", recorderID)
}
