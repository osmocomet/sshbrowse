package app

import (
	"testing"
	"time"
)

func TestOutputDeliveryWaitsForMatchingAcknowledgement(t *testing.T) {
	events := make(chan SessionData, 2)
	delivery := newOutputDelivery(time.Second, func(event SessionData) {
		events <- event
	})

	firstDone := make(chan bool, 1)
	go func() {
		firstDone <- delivery.deliver(7, "first")
	}()
	first := receiveSessionData(t, events)
	if first.ID != 7 || first.Sequence != 1 || first.Data != "first" {
		t.Fatalf("first event = %+v", first)
	}
	if err := delivery.acknowledge(2); err == nil {
		t.Fatal("wrong sequence acknowledged")
	}
	select {
	case <-firstDone:
		t.Fatal("delivery returned before acknowledgement")
	default:
	}
	if err := delivery.acknowledge(1); err != nil {
		t.Fatal(err)
	}
	if delivered := receiveBool(t, firstDone); !delivered {
		t.Fatal("acknowledged delivery stopped")
	}

	secondDone := make(chan bool, 1)
	go func() {
		secondDone <- delivery.deliver(7, "second")
	}()
	second := receiveSessionData(t, events)
	if second.Sequence != 2 || second.Data != "second" {
		t.Fatalf("second event = %+v", second)
	}
	if err := delivery.acknowledge(2); err != nil {
		t.Fatal(err)
	}
	if delivered := receiveBool(t, secondDone); !delivered {
		t.Fatal("second delivery stopped")
	}
}

func TestOutputDeliveryStopUnblocksWaiter(t *testing.T) {
	events := make(chan SessionData, 1)
	delivery := newOutputDelivery(time.Second, func(event SessionData) {
		events <- event
	})
	done := make(chan bool, 1)
	go func() {
		done <- delivery.deliver(1, "data")
	}()
	receiveSessionData(t, events)

	delivery.Stop()
	delivery.Stop()
	if delivered := receiveBool(t, done); delivered {
		t.Fatal("stopped delivery reported success")
	}
	if err := delivery.acknowledge(1); err == nil {
		t.Fatal("stopped delivery accepted an acknowledgement")
	}
	if delivery.deliver(1, "late") {
		t.Fatal("stopped delivery accepted more data")
	}
}

func TestOutputDeliveryAcknowledgementTimeoutIsBounded(t *testing.T) {
	events := make(chan SessionData, 1)
	delivery := newOutputDelivery(20*time.Millisecond, func(event SessionData) {
		events <- event
	})
	done := make(chan bool, 1)
	go func() {
		done <- delivery.deliver(1, "data")
	}()
	receiveSessionData(t, events)

	if delivered := receiveBool(t, done); delivered {
		t.Fatal("timed out delivery reported success")
	}
	if delivery.deliver(1, "late") {
		t.Fatal("timed out delivery accepted more data")
	}
}

func receiveSessionData(t *testing.T, events <-chan SessionData) SessionData {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for terminal output event")
		return SessionData{}
	}
}

func receiveBool(t *testing.T, result <-chan bool) bool {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivery")
		return false
	}
}
