package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	cspb "github.com/pascalhuerst/session-recorder/protocols/go/chunksink"
	cmpb "github.com/pascalhuerst/session-recorder/protocols/go/common"
)

/**
 * Test Plan: ChunkSink gRPC Server
 *
 * Scenario: Create new server with config
 *   Given a ChunkSinkServerConfig with name and version
 *   When NewChunkSinkServer is called
 *   Then a non-nil server is returned with initialized maps
 *
 * Scenario: SetRecorderStatus without callback
 *   Given a server with no OnRecorderStatusCB
 *   When SetRecorderStatus is called
 *   Then a success response is returned
 *
 * Scenario: SetRecorderStatus with callback
 *   Given a server with OnRecorderStatusCB configured
 *   When SetRecorderStatus is called
 *   Then the callback is invoked with the status
 *   And the result reflects callback success/failure
 *
 * Scenario: SetChunks without callback
 *   Given a server with no OnChunksCB
 *   When SetChunks is called
 *   Then a success response is returned
 *
 * Scenario: SetChunks with callback
 *   Given a server with OnChunksCB configured
 *   When SetChunks is called
 *   Then the callback is invoked with the chunks
 *   And the result reflects callback success/failure
 *
 * Scenario: CutSession without active connection
 *   Given a server with no active recorder connections
 *   When CutSession is called with a recorder ID
 *   Then an error is returned indicating no connection
 */

func TestNewChunkSinkServer(t *testing.T) {
	config := &ChunkSinkServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
	}

	server := NewChunkSinkServer(config)

	if server == nil {
		t.Error("NewChunkSinkServer() returned nil")
	}

	if server.config != config {
		t.Error("NewChunkSinkServer() config not set correctly")
	}

	if server.commandStreams == nil {
		t.Error("NewChunkSinkServer() commandStreams not initialized")
	}
}

func TestChunkSinkServer_SetRecorderStatus_NoCallback(t *testing.T) {
	server := NewChunkSinkServer(&ChunkSinkServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
	})

	status := &cmpb.RecorderStatus{
		RecorderID:   uuid.New().String(),
		RecorderName: "Test Recorder",
	}

	resp, err := server.SetRecorderStatus(context.Background(), status)
	if err != nil {
		t.Errorf("SetRecorderStatus() error = %v", err)
	}

	if resp == nil {
		t.Error("SetRecorderStatus() returned nil response")
		return
	}

	if !resp.Success {
		t.Error("SetRecorderStatus() response.Success = false, want true")
	}
}

func TestChunkSinkServer_SetRecorderStatus_WithCallback(t *testing.T) {
	callbackCalled := false
	expectedRecorderID := uuid.New().String()

	server := NewChunkSinkServer(&ChunkSinkServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
		OnRecorderStatusCB: func(ctx context.Context, status *cmpb.RecorderStatus) error {
			callbackCalled = true
			if status.RecorderID != expectedRecorderID {
				t.Errorf("callback RecorderID = %v, want %v", status.RecorderID, expectedRecorderID)
			}
			return nil
		},
	})

	status := &cmpb.RecorderStatus{
		RecorderID:   expectedRecorderID,
		RecorderName: "Test Recorder",
	}

	resp, err := server.SetRecorderStatus(context.Background(), status)
	if err != nil {
		t.Errorf("SetRecorderStatus() error = %v", err)
	}

	if !callbackCalled {
		t.Error("SetRecorderStatus() callback not called")
	}

	if !resp.Success {
		t.Error("SetRecorderStatus() response.Success = false, want true")
	}
}

func TestChunkSinkServer_SetRecorderStatus_CallbackError(t *testing.T) {
	expectedError := errors.New("callback error")

	server := NewChunkSinkServer(&ChunkSinkServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
		OnRecorderStatusCB: func(ctx context.Context, status *cmpb.RecorderStatus) error {
			return expectedError
		},
	})

	status := &cmpb.RecorderStatus{
		RecorderID:   uuid.New().String(),
		RecorderName: "Test Recorder",
	}

	resp, err := server.SetRecorderStatus(context.Background(), status)
	if err == nil {
		t.Error("SetRecorderStatus() error = nil, want error")
	}

	if resp == nil {
		t.Error("SetRecorderStatus() returned nil response")
		return
	}

	if resp.Success {
		t.Error("SetRecorderStatus() response.Success = true, want false")
	}

	if resp.ErrorMessage != expectedError.Error() {
		t.Errorf("SetRecorderStatus() error message = %v, want %v", resp.ErrorMessage, expectedError.Error())
	}
}

func TestChunkSinkServer_SetChunks_NoCallback(t *testing.T) {
	server := NewChunkSinkServer(&ChunkSinkServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
	})

	chunks := &cspb.Chunks{
		RecorderID: uuid.New().String(),
		SessionID:  uuid.New().String(),
		ChunkCount: 1,
		Data:       []byte{1, 2, 3, 4, 5, 6},
	}

	resp, err := server.SetChunks(context.Background(), chunks)
	if err != nil {
		t.Errorf("SetChunks() error = %v", err)
	}

	if resp == nil {
		t.Error("SetChunks() returned nil response")
		return
	}

	if !resp.Success {
		t.Error("SetChunks() response.Success = false, want true")
	}
}

func TestChunkSinkServer_SetChunks_WithCallback(t *testing.T) {
	callbackCalled := false
	expectedRecorderID := uuid.New().String()

	server := NewChunkSinkServer(&ChunkSinkServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
		OnChunksCB: func(ctx context.Context, chunks *cspb.Chunks) error {
			callbackCalled = true
			if chunks.RecorderID != expectedRecorderID {
				t.Errorf("callback RecorderID = %v, want %v", chunks.RecorderID, expectedRecorderID)
			}
			return nil
		},
	})

	chunks := &cspb.Chunks{
		RecorderID: expectedRecorderID,
		SessionID:  uuid.New().String(),
		ChunkCount: 1,
		Data:       []byte{1, 2, 3, 4, 5, 6},
	}

	resp, err := server.SetChunks(context.Background(), chunks)
	if err != nil {
		t.Errorf("SetChunks() error = %v", err)
	}

	if !callbackCalled {
		t.Error("SetChunks() callback not called")
	}

	if !resp.Success {
		t.Error("SetChunks() response.Success = false, want true")
	}
}

func TestChunkSinkServer_SetChunks_CallbackError(t *testing.T) {
	expectedError := errors.New("callback error")

	server := NewChunkSinkServer(&ChunkSinkServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
		OnChunksCB: func(ctx context.Context, chunks *cspb.Chunks) error {
			return expectedError
		},
	})

	chunks := &cspb.Chunks{
		RecorderID: uuid.New().String(),
		SessionID:  uuid.New().String(),
		ChunkCount: 1,
		Data:       []byte{1, 2, 3, 4, 5, 6},
	}

	resp, err := server.SetChunks(context.Background(), chunks)
	if err == nil {
		t.Error("SetChunks() error = nil, want error")
	}

	if resp == nil {
		t.Error("SetChunks() returned nil response")
		return
	}

	if resp.Success {
		t.Error("SetChunks() response.Success = true, want false")
	}
}

func TestChunkSinkServer_CutSession_NoConnection(t *testing.T) {
	server := NewChunkSinkServer(&ChunkSinkServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
	})

	err := server.CutSession(uuid.New())
	if err == nil {
		t.Error("CutSession() error = nil, want error for no connection")
	}
}

func TestChunkSinkServer_announcement(t *testing.T) {
	server := NewChunkSinkServer(&ChunkSinkServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
	})

	announcement := server.announcement()
	if len(announcement) != 2 {
		t.Errorf("announcement() returned %d items, want 2", len(announcement))
	}

	// Check that announcements contain expected strings
	if len(announcement) > 0 && string(announcement[0]) == "" {
		t.Error("announcement[0] is empty")
	}
	if len(announcement) > 1 && string(announcement[1]) == "" {
		t.Error("announcement[1] is empty")
	}
}

func TestChunkSinkServer_serverOptions(t *testing.T) {
	server := NewChunkSinkServer(&ChunkSinkServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
	})

	options := server.serverOptions()
	if options == nil {
		t.Error("serverOptions() returned nil")
	}
}

// fakeCommandStream is a GetCommands server stream that ends when its
// context is cancelled and records what is sent on it.
type fakeCommandStream struct {
	cspb.ChunkSink_GetCommandsServer
	ctx  context.Context
	sent []*cspb.Command
}

func (f *fakeCommandStream) Context() context.Context { return f.ctx }

func (f *fakeCommandStream) Send(c *cspb.Command) error {
	f.sent = append(f.sent, c)
	return nil
}

// openStream runs GetCommands for recorderID until cancel is called; done
// is closed once GetCommands has returned.
func openStream(t *testing.T, s *ChunkSinkServer, recorderID uuid.UUID) (*fakeCommandStream, context.CancelFunc, chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stream := &fakeCommandStream{ctx: ctx}
	s.mu.Lock()
	ownID := s.nextStreamID + 1
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		_ = s.GetCommands(&cspb.GetCommandRequest{RecorderID: recorderID.String()}, stream)
		close(done)
	}()
	// Wait for this stream, not just any: with an older stream already
	// registered, checking for an entry would return before this one is.
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		registered := s.commandStreams[recorderID].id == ownID
		s.mu.Unlock()
		if registered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stream was not registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return stream, cancel, done
}

// A stale stream closing after the recorder opened a newer one must not
// unregister the newer one (seen 2026-10-17/18: CutSession then failed with
// "no connection to recorder" while the recorder was connected).
func TestChunkSinkServer_StaleStreamCloseKeepsNewerStream(t *testing.T) {
	s := NewChunkSinkServer(&ChunkSinkServerConfig{Name: "test"})
	recorderID := uuid.New()

	_, cancelOld, oldDone := openStream(t, s, recorderID)
	newer, cancelNew, newDone := openStream(t, s, recorderID)

	cancelOld()
	<-oldDone
	if err := s.CutSession(recorderID); err != nil {
		t.Fatalf("CutSession after the stale stream closed: %v", err)
	}
	if len(newer.sent) != 1 {
		t.Fatalf("the newer stream got %d commands, want 1", len(newer.sent))
	}

	cancelNew()
	<-newDone
	if err := s.CutSession(recorderID); err == nil {
		t.Fatal("CutSession with no stream left should fail")
	}
}
