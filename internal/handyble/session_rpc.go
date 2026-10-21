package handyble

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

func (s *Session) rpc(ctx context.Context, generation uint64, request Request, wait bool) (Message, error) {
	if err := s.valid(generation); err != nil {
		return Message{}, err
	}
	if err := ctx.Err(); err != nil {
		return Message{}, err
	}
	s.mu.Lock()
	// IDs never wrap on a live connection: old notifications remain distinguishable.
	if s.nextID == math.MaxUint32-1 {
		s.mu.Unlock()
		return Message{}, errors.New("handy request IDs exhausted; reconnect")
	}
	s.nextID++
	request.ID = s.nextID
	reply := make(chan Message, 1)
	s.recent[request.ID] = pendingRequest{generation: generation, method: request.Method, reply: reply}
	if request.ID > maxRecentRequests {
		delete(s.recent, request.ID-maxRecentRequests)
	}
	s.mu.Unlock()
	data, err := Encode(request)
	if err != nil {
		return Message{}, err
	}
	if len(data) > s.limit {
		return Message{}, errors.New("handy RPC exceeds the negotiated Bluetooth write size")
	}
	// Complete an already-submitted write within its fixed deadline so an
	// enclosing cancellation leaves a healthy link available for the next Stop.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	err = s.link.Write(writeCtx, data, true)
	cancel()
	if err != nil {
		s.fail(err)
		return Message{}, err
	}
	if err := s.valid(generation); err != nil {
		return Message{}, err
	}
	if err := ctx.Err(); err != nil {
		return Message{}, err
	}
	if !wait {
		select {
		case response := <-reply:
			return response, responseError(response)
		default:
			return Message{ID: request.ID, Method: request.Method}, nil
		}
	}
	timer := time.NewTimer(responseTimeout)
	defer timer.Stop()
	select {
	case response := <-reply:
		return response, responseError(response)
	case <-ctx.Done():
		return Message{}, ctx.Err()
	case <-s.quit:
		return Message{}, errors.New("handy Bluetooth session ended")
	case <-timer.C:
		return Message{}, errors.New("handy did not return an RPC response")
	}
}

func responseError(message Message) error {
	if message.ErrorCode != nil {
		return fmt.Errorf("handy rejected RPC (code %d)", *message.ErrorCode)
	}
	return nil
}

func (s *Session) syncClock(ctx context.Context) error {
	var bestOffset int64
	bestRTT := time.Duration(math.MaxInt64)
	for range 3 {
		before := time.Now()
		response, err := s.Execute(ctx, Request{Method: ClockGet})
		after := time.Now()
		if err != nil {
			return err
		}
		rtt := after.Sub(before)
		wall := after.UnixMilli() - before.UnixMilli()
		if response.DeviceTime == nil || absMillis(wall-rtt.Milliseconds()) >= 100 {
			continue
		}
		if rtt < bestRTT {
			bestRTT = rtt
			bestOffset = (before.UnixMilli()+after.UnixMilli())/2 - int64(*response.DeviceTime)
		}
	}
	if bestRTT == time.Duration(math.MaxInt64) {
		return errors.New("no usable Handy clock samples")
	}
	rttMillis := bestRTT.Milliseconds()
	if rttMillis < 0 || rttMillis > math.MaxInt32 {
		return errors.New("invalid Handy clock round trip")
	}
	if _, err := s.Execute(ctx, Request{Method: ClockSet, ClockOffset: bestOffset, RoundTrip: int32(rttMillis)}); err != nil {
		return err
	}
	s.mu.Lock()
	s.clockSynced = s.err == nil
	s.mu.Unlock()
	return nil
}

func absMillis(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
