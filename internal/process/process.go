package process

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

type Result struct {
	ExitCode       int
	Stdout, Stderr string
	Duration       time.Duration
}

func Run(ctx context.Context, dir, name string, args ...string) (Result, error) {
	start := time.Now()
	c := exec.CommandContext(ctx, name, args...)
	c.Dir = dir
	var out, er bytes.Buffer
	c.Stdout = &limited{&out, 1 << 20}
	c.Stderr = &limited{&er, 1 << 20}
	e := c.Run()
	r := Result{0, out.String(), er.String(), time.Since(start)}
	if e != nil {
		if x, ok := e.(*exec.ExitError); ok {
			r.ExitCode = x.ExitCode()
		} else {
			r.ExitCode = -1
		}
	}
	return r, e
}

type limited struct {
	b *bytes.Buffer
	n int
}

func (w *limited) Write(p []byte) (int, error) {
	original := len(p)
	if w.b.Len() < w.n {
		left := w.n - w.b.Len()
		if len(p) > left {
			p = p[:left]
		}
		_, _ = w.b.Write(p)
	}
	return original, nil
}
