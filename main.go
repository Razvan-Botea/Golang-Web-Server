package main

import (
    "net/http"
)

func main() {
    mux := http.NewServeMux()

    mux.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("."))))
    mux.Handle("/app/assets/", http.StripPrefix("/app/assets/", http.FileServer(http.Dir("./assets"))))
    mux.HandleFunc("/healthz", readinessHandler)

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
