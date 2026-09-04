package router

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestServeWithContextStopsOnCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	server := &http.Server{Handler: http.NewServeMux()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveWithContext(ctx, server, listener)
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveWithContext returned %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serveWithContext did not stop after context cancellation")
	}

	if err := server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("server close: %v", err)
	}
}

func TestHTTPServerUsesBoundedUploadCompatibleTimeouts(t *testing.T) {
	server := newHTTPServer(":8080", http.NewServeMux())
	if server.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != maxRequestReadDuration {
		t.Fatalf("ReadTimeout = %v, want %v", server.ReadTimeout, maxRequestReadDuration)
	}
	if server.WriteTimeout != maxUploadWriteDuration {
		t.Fatalf("WriteTimeout = %v, want %v", server.WriteTimeout, maxUploadWriteDuration)
	}
	if server.IdleTimeout != defaultIdleConnectionTTL {
		t.Fatalf("IdleTimeout = %v, want %v", server.IdleTimeout, defaultIdleConnectionTTL)
	}
}

func TestShutdownSignalsIncludeInterrupt(t *testing.T) {
	for _, candidate := range shutdownSignals() {
		if candidate == os.Interrupt {
			return
		}
	}
	t.Fatal("shutdown signals must include os.Interrupt")
}
