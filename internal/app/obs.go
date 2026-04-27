package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"racoo.cn/lsp/internal/store/redis"
)

var registerRuntimeCollectorsOnce sync.Once

// StartObsHTTP 若 addr 非空则启动可观测性 HTTP 服务（健康检查、就绪检查、指标、pprof）。
func StartObsHTTP(addr string, rcli *redis.Client) (stop func(), err error) {
	if addr == "" {
		return func() {}, nil
	}
	registerRuntimeCollectorsOnce.Do(registerRuntimeCollectors)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if rcli != nil {
			pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := rcli.Ping(pingCtx); err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}
	go func() { _ = srv.ListenAndServe() }()
	return func() {
		shCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}, nil
}

func registerRuntimeCollectors() {
	registerCollector(collectors.NewGoCollector())
	registerCollector(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

func registerCollector(c prometheus.Collector) {
	if err := prometheus.Register(c); err != nil {
		var already prometheus.AlreadyRegisteredError
		if !errors.As(err, &already) {
			return
		}
	}
}
