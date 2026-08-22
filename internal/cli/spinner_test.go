package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestSpinnerRendersAndStopsIdempotently(t *testing.T) {
	var output bytes.Buffer
	s := newSpinner(context.Background(), &output, "Waiting", time.Millisecond, true)
	s.Start()
	time.Sleep(10 * time.Millisecond)
	s.Stop()
	s.Stop()

	got := output.String()
	if !strings.Contains(got, "Waiting") {
		t.Fatalf("spinner output %q does not contain its message", got)
	}
	if !strings.HasSuffix(got, "\r\x1b[2K") {
		t.Fatalf("spinner output %q was not cleared", got)
	}

	length := output.Len()
	time.Sleep(5 * time.Millisecond)
	if output.Len() != length {
		t.Fatal("spinner wrote output after Stop returned")
	}
}

func TestSpinnerCancellationStopsRendering(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var output bytes.Buffer
	s := newSpinner(ctx, &output, "Waiting", time.Millisecond, true)
	s.Start()
	cancel()
	s.Stop()

	length := output.Len()
	time.Sleep(5 * time.Millisecond)
	if output.Len() != length {
		t.Fatal("spinner wrote output after cancellation and Stop")
	}
}

func TestSpinnerNonInteractiveWritesNothing(t *testing.T) {
	var output bytes.Buffer
	s := newSpinner(context.Background(), &output, "Waiting", time.Millisecond, false)
	s.Start()
	time.Sleep(5 * time.Millisecond)
	s.Stop()
	if output.Len() != 0 {
		t.Fatalf("non-interactive spinner wrote %q", output.String())
	}
}

func TestWaitForNextCycleStopsSpinner(t *testing.T) {
	var output bytes.Buffer
	ticks := make(chan time.Time, 1)
	ticks <- time.Now()
	s := newSpinner(context.Background(), &output, "Waiting", time.Hour, true)

	if !waitForNextCycle(context.Background(), ticks, s) {
		t.Fatal("wait did not report the polling tick")
	}
	length := output.Len()
	time.Sleep(5 * time.Millisecond)
	if output.Len() != length {
		t.Fatal("spinner wrote after polling wait completed")
	}
}

func TestWaitForNextCycleCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	s := newSpinner(ctx, &output, "Waiting", time.Hour, true)

	if waitForNextCycle(ctx, make(chan time.Time), s) {
		t.Fatal("cancelled wait reported a polling tick")
	}
}
