package httputil

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

func ListenAndServe(
	ctx context.Context,
	bindAddr string,
	handler http.Handler,
) error {
	slog.Info("starting HTTP server", "bind-addr", bindAddr)
	li, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", bindAddr, err)
	}
	defer li.Close()

	srv := &http.Server{
		Handler: handler,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
		ReadHeaderTimeout: time.Minute,
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down HTTP server")
		sctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	err = srv.Serve(li)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to serve HTTP: %w", err)
	}

	return nil
}
