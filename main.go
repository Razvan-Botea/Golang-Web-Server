package main

import (
    "net/http"
    "sync/atomic"
    "fmt"
    "encoding/json"
    "log"
    "strings"
    "database/sql"
    "os"
    "time"

    "github.com/google/uuid"
    _ "github.com/lib/pq"
    "github.com/joho/godotenv"

    "web-server/internal/database"
)

type apiConfig struct {
    fileserverHits atomic.Int32
    dbQueries *database.Queries
    platform string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
    ID uuid.UUID `json:"id"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    Body string `json:"body"`
    UserID uuid.UUID `json:"user_id"`
}

func main() {
    err := godotenv.Load()
    if err != nil {
        log.Println("Warning: No .env file found")
    }

    dbURL := os.Getenv("DB_URL")
    platform := os.Getenv("PLATFORM")
    db, err := sql.Open("postgres", dbURL)
    if err != nil {
        log.Fatal("Error opening database: ", err)
    }
    defer db.Close()

    mux := http.NewServeMux()
    apiCfg := &apiConfig{}

    apiCfg.dbQueries = database.New(db)
    apiCfg.platform = platform

    fileServerHandler := http.StripPrefix("/app/", http.FileServer(http.Dir(".")))
    mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileServerHandler))
    mux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
    mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
    mux.HandleFunc("GET /api/healthz", readinessHandler)
    mux.HandleFunc("POST /api/users", apiCfg.createUserHandler)
    mux.HandleFunc("POST /api/chirps", apiCfg.chirpHandler)

    server := &http.Server{
        Addr: ":8080",
        Handler: mux,
    }

    server.ListenAndServe()
}

func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
    type parameters struct {
        Email string `json:"email"`
    }

    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err := decoder.Decode(&params)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
        return
    }

    user, err := cfg.dbQueries.CreateUser(r.Context(), params.Email)
    if err != nil {
        log.Printf("CreateUser error: %s", err)
        respondWithError(w, http.StatusInternalServerError, "Couldn't create user")
        return
    }

    respondWithJson(w, http.StatusCreated, User{
        ID: user.ID,
        CreatedAt: user.CreatedAt,
        UpdatedAt: user.UpdatedAt,
        Email: user.Email,
    })
}

func (cfg *apiConfig) chirpHandler(w http.ResponseWriter, r *http.Request) {
    type parameters struct {
        Body string `json:"body"`
        User_ID uuid.UUID `json:"user_id"`
    }

    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err := decoder.Decode(&params)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
        return
    }

    const maxChirpLen = 140
    if maxChirpLen < len(params.Body) {
        respondWithError(w, http.StatusBadRequest, "Chirp is too long")
        return
    }

    cleaned := replaceBadWords(params.Body)

    chirp, err := cfg.dbQueries.CreateChirp(r.Context(), database.CreateChirpParams{
        Body: cleaned,
        UserID: params.User_ID,
    })
    if err != nil {
        log.Printf("CreateChirp error: %s", err)
        respondWithError(w, http.StatusInternalServerError, "Couldn't create chirp")
        return
    }
    
    respondWithJson(w, http.StatusCreated, Chirp{
        ID: chirp.ID,
        CreatedAt: chirp.CreatedAt,
        UpdatedAt: chirp.UpdatedAt,
        Body: chirp.Body,
        UserID: chirp.UserID,
    })
}

func replaceBadWords(text string) string {
    replacement := "****"
    words := strings.Split(text, " ")
    for i := 0; i < len(words); i++ {
        switch strings.ToLower(words[i]) {
            case "kerfuffle":
                words[i] = replacement
            case "sharbert":
                words[i] = replacement
            case "fornax":
                words[i] = replacement
        }
    }
    result := strings.Join(words, " ")
    return result
}

func respondWithJson (w http.ResponseWriter, code int, payload interface{}) {
    response, err := json.Marshal(payload)
    if err != nil {
        log.Printf("Error marshalling JSON: %s", err)
        w.WriteHeader(http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    w.Write(response)
}

func respondWithError (w http.ResponseWriter, code int, msg string) {
    type errorResponse struct {
        Error string `json:"error"`
    }

    respondWithJson(w, code, errorResponse{Error: msg,})
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
    if cfg.platform != "dev" {
        w.WriteHeader(http.StatusForbidden)
        w.Write([]byte("Reset is only allowed in dev environment"))
        return
    }

    cfg.fileserverHits.Store(0)

    err := cfg.dbQueries.DeleteUsers(r.Context())
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "Couldn't delete users")
        return
    }

    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}
