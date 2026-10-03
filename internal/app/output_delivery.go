package app

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const outputAcknowledgementTimeout = 30 * time.Second

// outputDelivery keeps exactly one terminal write in flight. deliver is called
// only by the session's PTY reader, so acknowledging a sequence also preserves
// byte order for the next read.
type outputDelivery struct {
	mu       sync.Mutex
	stopped  chan struct{}
	stopOnce sync.Once

	nextSequence int
	awaiting     int
	acknowledged chan struct{}
	timeout      time.Duration
	emit         func(SessionData)
}

func newOutputDelivery(timeout time.Duration, emit func(SessionData)) *outputDelivery {
	return &outputDelivery{
		stopped: make(chan struct{}),
		timeout: timeout,
		emit:    emit,
	}
}

func (d *outputDelivery) deliver(id int, data string) bool {
	d.mu.Lock()
	select {
	case <-d.stopped:
		d.mu.Unlock()
		return false
	default:
	}
	d.nextSequence++
	sequence := d.nextSequence
	acknowledged := make(chan struct{})
	d.awaiting = sequence
	d.acknowledged = acknowledged
	d.mu.Unlock()

	d.emit(SessionData{ID: id, Sequence: sequence, Data: data})

	timer := time.NewTimer(d.timeout)
	defer timer.Stop()
	select {
	case <-acknowledged:
		return true
	case <-d.stopped:
		return false
	case <-timer.C:
		d.Stop()
		return false
	}
}

func (d *outputDelivery) acknowledge(sequence int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	select {
	case <-d.stopped:
		return errors.New("terminal output delivery stopped")
	default:
	}
	if d.acknowledged == nil || d.awaiting != sequence {
		return fmt.Errorf("terminal output sequence %d is not awaiting acknowledgement", sequence)
	}
	close(d.acknowledged)
	d.acknowledged = nil
	d.awaiting = 0
	return nil
}

func (d *outputDelivery) Stop() {
	d.stopOnce.Do(func() {
		close(d.stopped)
	})
}
