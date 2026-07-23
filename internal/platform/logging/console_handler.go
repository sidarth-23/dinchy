package logging

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

// consoleHandler wraps a tint text handler and renders selected attributes (e.g.
// multi-line SQL) as indented trailing lines instead of tint's escaped
// single-line quoting, keeping local text output readable. Text/dev only; the
// JSON handler used in production is unaffected.
type consoleHandler struct {
	mu        *sync.Mutex
	out       io.Writer
	buf       *bytes.Buffer
	inner     slog.Handler
	blockKeys map[string]bool
}

func newConsoleHandler(out io.Writer, level slog.Level, addSource bool, blockKeys map[string]bool) *consoleHandler {
	buf := &bytes.Buffer{}
	inner := tint.NewTextHandler(buf, &tint.Options{
		Level:      level,
		AddSource:  addSource,
		TimeFormat: "15:04:05.000",
		NoColor:    consoleNoColor(out),
	})
	return &consoleHandler{mu: &sync.Mutex{}, out: out, buf: buf, inner: inner, blockKeys: blockKeys}
}

func consoleNoColor(w io.Writer) bool {
	if file, ok := w.(*os.File); ok {
		return !isatty.IsTerminal(file.Fd())
	}
	return true
}

func (h *consoleHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *consoleHandler) Handle(ctx context.Context, record slog.Record) error {
	var blocks []string
	line := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		if h.blockKeys[attr.Key] {
			blocks = append(blocks, attr.Value.String())
			return true
		}
		line.AddAttrs(attr)
		return true
	})

	h.mu.Lock()
	defer h.mu.Unlock()

	h.buf.Reset()
	if err := h.inner.Handle(ctx, line); err != nil {
		return err
	}
	if _, err := h.out.Write(bytes.TrimRight(h.buf.Bytes(), "\n")); err != nil {
		return err
	}
	for _, block := range blocks {
		for blockLine := range strings.SplitSeq(block, "\n") {
			if _, err := io.WriteString(h.out, "\n  "+blockLine); err != nil {
				return err
			}
		}
	}
	_, err := io.WriteString(h.out, "\n")
	return err
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &consoleHandler{mu: h.mu, out: h.out, buf: h.buf, inner: h.inner.WithAttrs(attrs), blockKeys: h.blockKeys}
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	return &consoleHandler{mu: h.mu, out: h.out, buf: h.buf, inner: h.inner.WithGroup(name), blockKeys: h.blockKeys}
}
