package main

import (
    "net/http"
    "sync/atomic"
    "fmt"
)

type apiConfig struct {
    fileserverHits atomic.Int32
}

func main() {
    mux := http.NewServeMux()
    apiCfg := &apiConfig{}

    fileServerHandler := http.StripPrefix("/app/", http.FileServer(http.Dir(".")))
    mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileServerHandler))
    mux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
    mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
    mux.HandleFunc("GET /api/healthz", readinessHandler)

    server := &http.Server{
        Addr: ":8080",
        Handler: mux,
    }

    server.ListenAndServe()
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        cfg.fileserverHits.Add(1)
        next.ServeHTTP(w, r)
    })
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(http.StatusOK)

    noRequests := cfg.fileserverHits.Load()
    w.Write([]byte(fmt.Sprintf("<html> <body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", noRequests)))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
    cfg.fileserverHits.Store(0)

    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}
