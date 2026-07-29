package nats

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	flags := log.Flags()
	log.SetOutput(buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flags)
	})
	return buf
}

func TestDispatchLogsHandlerError(t *testing.T) {
	buf := captureLog(t)

	dispatch(context.Background(), func(context.Context, string, []byte) error {
		return errors.New("telegram api status 500")
	}, "order.paid", []byte(`{}`))

	logged := buf.String()
	if !strings.Contains(logged, "order.paid") {
		t.Fatalf("log output %q does not name the subject", logged)
	}
	if !strings.Contains(logged, "telegram api status 500") {
		t.Fatalf("log output %q does not include the handler error", logged)
	}
}

func TestDispatchSilentOnSuccess(t *testing.T) {
	buf := captureLog(t)

	dispatch(context.Background(), func(context.Context, string, []byte) error {
		return nil
	}, "order.paid", []byte(`{}`))

	if buf.Len() != 0 {
		t.Fatalf("log output = %q, want empty on success", buf.String())
	}
}

func TestDispatchNilHandler(t *testing.T) {
	buf := captureLog(t)

	dispatch(context.Background(), nil, "order.paid", []byte(`{}`))

	if buf.Len() != 0 {
		t.Fatalf("log output = %q, want empty for nil handler", buf.String())
	}
}
