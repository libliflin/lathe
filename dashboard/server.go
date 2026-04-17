package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// Serve is the hidden `_dashboard_serve` entry point, called via re-exec from `cmdStart`.
// It owns the HTTP server for the full lifetime of the dashboard daemon.
func Serve(args []string) {
	host := "127.0.0.1"
	port := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			i++
			if i < len(args) {
				host = args[i]
			}
		case "--port":
			i++
			if i < len(args) {
				p, err := strconv.Atoi(args[i])
				if err != nil {
					fmt.Fprintf(os.Stderr, "bad --port: %v\n", err)
					os.Exit(1)
				}
				port = p
			}
		}
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bind: %v\n", err)
		os.Exit(1)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	writeDaemonState(&daemonState{PID: os.Getpid(), Host: host, Port: actualPort})

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/state", handleState)
	mux.HandleFunc("/api/stream", handleStream)

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE streams need long writes
	}

	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		}
	}()

	fmt.Fprintf(os.Stderr, "dashboard listening on http://%s:%d\n", host, actualPort)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	<-sigs

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	clearDaemonState()
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(indexHTML)
}

func handleState(w http.ResponseWriter, r *http.Request) {
	snap := Collect()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(snap)
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering just in case

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	send := func() bool {
		snap := Collect()
		data, err := json.Marshal(snap)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !send() {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}
