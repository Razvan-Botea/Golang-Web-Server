package main

import (
    "net/http"
)

func main() {
    handler := http.NewServeMux()
    server := &http.Server{
        Addr: ":8080",
        Handler: handler,
    }

    server.ListenAndServe()
}
