package httpserver

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-errors/errors"
)

func TestServeUntil_CancellationIsClean(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv := &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: time.Second,
	}
	if err := ServeUntil(ctx, srv, time.Second); err != nil {
		t.Fatalf("ServeUntil: %v", err)
	}
}

func TestServeUntil_OccupiedAddressIsFatal(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: time.Second,
	}
	err = ServeUntil(context.Background(), srv, time.Second)
	if err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("error = %v, want listener failure", err)
	}
}

func TestServeUntil_ShutdownDeadlineForcesCloseAndJoins(t *testing.T) {
	addr := unusedAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handlerStarted := make(chan struct{})
	handlerRelease := make(chan struct{})
	handlerDone := make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(handlerRelease) }) }
	t.Cleanup(releaseHandler)

	srv := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			close(handlerStarted)
			<-handlerRelease
			close(handlerDone)
		}),
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- ServeUntil(ctx, srv, 20*time.Millisecond)
	}()
	waitForListener(t, addr, serveDone)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	requestDone := make(chan error, 1)
	go func() {
		resp, err := (&http.Client{Timeout: time.Second}).Do(req)
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-handlerStarted:
	case err := <-serveDone:
		t.Fatalf("ServeUntil returned before handler started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	cancel()
	select {
	case err := <-serveDone:
		if err == nil || !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "shutdown") {
			t.Fatalf("ServeUntil error = %v, want contextual shutdown deadline", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ServeUntil did not force-close and join the listener")
	}

	// Returning only after the Serve child is joined means its listener has been released.
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatalf("listener was not released after ServeUntil returned: %v", err)
	}
	_ = ln.Close()

	releaseHandler()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("test handler did not exit")
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("test request did not exit")
	}
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func waitForListener(t *testing.T, addr string, serveDone <-chan error) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn, err := (&net.Dialer{Timeout: 10 * time.Millisecond}).DialContext(t.Context(), "tcp", addr)
		if err == nil {
			_ = conn.Close()
			return
		}
		select {
		case err := <-serveDone:
			t.Fatalf("ServeUntil returned before listening: %v", err)
		default:
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("server did not listen on %s", addr)
}
