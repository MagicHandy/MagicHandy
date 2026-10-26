package voice

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Model changes own the protocol reader in some adapters. Keep their pending
// state in the supervisor, coalesce matching callers, and fence older probes
// and operations so they cannot overwrite a replacement process's status.
type modelOperation struct {
	loaded      bool
	cancel      context.CancelFunc
	done        chan struct{}
	response    Response
	err         error
	interrupted error
}

// Health returns the current model transition immediately; otherwise it probes
// the worker. A page polling during a cold load must not queue a doomed probe.
func (s *Supervisor) Health(ctx context.Context) (Response, error) {
	s.mu.Lock()
	workerConn, revision := s.conn, s.modelRevision
	if s.modelOperation != nil {
		response := s.lastHealth
		s.mu.Unlock()
		return response, nil
	}
	s.mu.Unlock()
	if workerConn == nil {
		return Response{}, fmt.Errorf("%s worker is not running", s.role)
	}
	response, err := s.roundTrip(ctx, workerConn, Request{Type: RequestHealth}, controlTimeout)
	if err == nil {
		switch response.Type {
		case ResponseError:
			if response.Error == nil {
				err = errors.New("voice worker health check failed")
			} else {
				err = response.Error
			}
		case ResponseHealth:
		default:
			err = fmt.Errorf("%s worker returned %q to health, want %q", s.role, response.Type, ResponseHealth)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != workerConn || s.stopping {
		return Response{}, context.Canceled
	}
	if s.modelRevision != revision {
		return s.lastHealth, nil
	}
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.lastHealth = Response{}
			s.lastError = err.Error()
		}
	} else {
		s.lastHealth, s.lastError = response, ""
	}
	return response, err
}

const modelLoadTimeout = 15 * time.Minute

// SetModelLoaded asks the worker to load or unload. Equal concurrent requests
// share the operation; an opposing request must wait or stop the worker first.
func (s *Supervisor) SetModelLoaded(ctx context.Context, loaded bool) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	s.mu.Lock()
	workerConn := s.conn
	if workerConn == nil || s.stopping {
		s.mu.Unlock()
		return Response{}, fmt.Errorf("%s worker is not running", s.role)
	}
	if operation := s.modelOperation; operation != nil {
		s.mu.Unlock()
		if operation.loaded != loaded {
			return Response{}, errors.New("voice model is already changing; wait or stop the worker")
		}
		return waitModelOperation(ctx, operation)
	}
	wantState := ModelStateUnloaded
	if loaded {
		wantState = ModelStateReady
	}
	if s.lastHealth.ModelState == wantState && s.lastError == "" {
		response := s.lastHealth
		s.mu.Unlock()
		return response, nil
	}
	timeout := controlTimeout
	if loaded {
		timeout = modelLoadTimeout
	}
	operationCtx, cancel := context.WithTimeout(context.Background(), timeout)
	operation := &modelOperation{loaded: loaded, cancel: cancel, done: make(chan struct{})}
	s.modelOperation = operation
	s.modelRevision++
	s.lastError = ""
	s.lastHealth = Response{Type: ResponseHealth, ModelState: ModelStateLoading}
	if !loaded {
		s.lastHealth.ModelState = "unloading"
	}
	s.mu.Unlock()
	go s.runModelOperation(operationCtx, workerConn, operation, timeout)
	return waitModelOperation(ctx, operation)
}

func waitModelOperation(ctx context.Context, operation *modelOperation) (Response, error) {
	select {
	case <-operation.done:
		return operation.response, operation.err
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}
}

func (s *Supervisor) runModelOperation(ctx context.Context, workerConn *conn, operation *modelOperation, timeout time.Duration) {
	defer operation.cancel()
	loaded := operation.loaded
	requestType := RequestUnload
	if loaded {
		requestType = RequestLoad
	}
	response, err := s.roundTrip(ctx, workerConn, Request{Type: requestType}, timeout)
	wantState := ModelStateUnloaded
	if loaded {
		wantState = ModelStateReady
	}
	if err == nil {
		if response.Type == ResponseError && response.Error != nil {
			err = response.Error
		} else if response.Type != ResponseHealth || response.ModelState != wantState {
			err = fmt.Errorf("%s worker returned %q with model state %q, want health/%s", s.role, response.Type, response.ModelState, wantState)
		}
	}
	s.mu.Lock()
	if s.conn == workerConn && s.modelOperation == operation && !s.stopping {
		s.modelOperation = nil
		s.modelRevision++
		if err != nil {
			s.lastHealth = Response{}
			s.lastError = err.Error()
		} else {
			s.lastHealth, s.lastError = response, ""
		}
	} else {
		response, err = Response{}, operation.interrupted
		if err == nil {
			err = context.Canceled
		}
	}
	operation.response, operation.err = response, err
	close(operation.done)
	s.mu.Unlock()
	// A timed-out operation cannot remain alive in a protocol reader. Retire
	// its owned session; the connection monitor contains the child process.
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		workerConn.closeWithError(ctx.Err())
	}
}
