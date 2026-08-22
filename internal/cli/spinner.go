package cli

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

var spinnerFrames = []string{"|", "/", "-", "\\"}

type spinner struct {
	ctx      context.Context
	writer   io.Writer
	message  string
	interval time.Duration
	enabled  bool

	startOnce sync.Once
	stopOnce  sync.Once
	stop      chan struct{}
	done      chan struct{}
}

func newSpinner(ctx context.Context, writer io.Writer, message string, interval time.Duration, enabled bool) *spinner {
	return &spinner{
		ctx:      ctx,
		writer:   writer,
		message:  message,
		interval: interval,
		enabled:  enabled,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (s *spinner) Start() {
	s.startOnce.Do(func() {
		if !s.enabled {
			close(s.done)
			return
		}
		go s.render()
	})
}

func (s *spinner) Stop() {
	s.Start()
	s.stopOnce.Do(func() { close(s.stop) })
	<-s.done
}

func (s *spinner) render() {
	defer close(s.done)
	defer fmt.Fprint(s.writer, "\r\x1b[2K")

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	frame := 0
	for {
		fmt.Fprintf(s.writer, "\r%s %s", spinnerFrames[frame], s.message)
		frame = (frame + 1) % len(spinnerFrames)
		select {
		case <-s.ctx.Done():
			return
		case <-s.stop:
			return
		case <-ticker.C:
		}
	}
}

type fileDescriptor interface {
	Fd() uintptr
}

func isTerminal(writer io.Writer) bool {
	file, ok := writer.(fileDescriptor)
	return ok && (isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd()))
}

func waitForNextCycle(ctx context.Context, ticks <-chan time.Time, spinner *spinner) bool {
	spinner.Start()
	defer spinner.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-ticks:
		return true
	}
}
