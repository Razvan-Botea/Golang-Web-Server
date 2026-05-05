package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	internal "web-server/internal/auth"
	"web-server/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
	jwtSecret      string
	polkaKey       string
}

type User struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	IsChirpyRed bool      `json:"is_chirpy_red"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: No .env file found")
	}

	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	secret := os.Getenv("JWT_SECRET")
	polka := os.Getenv("POLKA_KEY")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Error opening database: ", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	apiCfg := &apiConfig{}

	apiCfg.dbQueries = database.New(db)
	apiCfg.platform = platform
	apiCfg.jwtSecret = secret
	apiCfg.polkaKey = polka

	fileServerHandler := http.StripPrefix("/app/", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileServerHandler))
	mux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
	mux.HandleFunc("GET /api/healthz", readinessHandler)
	mux.HandleFunc("POST /api/users", apiCfg.createUserHandler)
	mux.HandleFunc("POST /api/chirps", apiCfg.chirpHandler)
	mux.HandleFunc("GET /api/chirps", apiCfg.getChirpsHandler)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getSingleChirpHandler)
	mux.HandleFunc("POST /api/login", apiCfg.loginHandler)
	mux.HandleFunc("POST /api/refresh", apiCfg.refreshHandler)
	mux.HandleFunc("POST /api/revoke", apiCfg.revokeHandler)
	mux.HandleFunc("PUT /api/users", apiCfg.updateUserHandler)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.deleteChirpHandler)
	mux.HandleFunc("POST /api/polka/webhooks", apiCfg.UpgradeUser)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	server.ListenAndServe()
}

func (cfg *apiConfig) UpgradeUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		}
	}

	apiKey, err := internal.GetAPIKey(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
		return
	}

	if apiKey != cfg.polkaKey {
		respondWithError(w, 401, "Invalid API key")
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	if params.Event != "user.upgraded" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	userID, err := uuid.Parse(params.Data.UserID)
	if err != nil {
		respondWithError(w, 204, "Invalid user ID")
		return
	}

	err = cfg.dbQueries.UpgradeUser(r.Context(), userID)
	if err != nil {
		respondWithError(w, 404, "Couldn't upgrade user")
		return
	}

	respondWithJson(w, 204, User{})
}

func (cfg *apiConfig) deleteChirpHandler(w http.ResponseWriter, r *http.Request) {
	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid chirp ID")
		return
	}

	tokenString, err := internal.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Missing or invalid Authorization header")
		return
	}

	userID, err := internal.ValidateJWT(tokenString, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, 403, "Invalid token")
		return
	}

	chirp, err := cfg.dbQueries.GetSingleChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, 404, "Couldn't find chirp")
		return
	}

	if userID != chirp.UserID {
		respondWithError(w, 403, "You can only delete your own chirps")
		return
	}
	err = cfg.dbQueries.DeleteChirp(r.Context(), database.DeleteChirpParams{
		ID:     chirpID,
		UserID: userID,
	})
	if err != nil {
		respondWithError(w, 404, "Couldn't delete chirp")
		return
	}

	respondWithJson(w, 204, Chirp{})
}

func (cfg *apiConfig) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	tokenString, err := internal.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Missing or invalid Authorization header")
		return
	}

	userID, err := internal.ValidateJWT(tokenString, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, 401, "Invalid token")
		return
	}

	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	hashedPassword, err := internal.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't hash password")
		return
	}

	updatedUser, err := cfg.dbQueries.UpdateUser(r.Context(), database.UpdateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
		ID:             userID,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update user")
		return
	}

	respondWithJson(w, http.StatusOK, User{
		ID:        updatedUser.ID,
		CreatedAt: updatedUser.CreatedAt,
		UpdatedAt: updatedUser.UpdatedAt,
		Email:     updatedUser.Email,
	})
}

func (cfg *apiConfig) refreshHandler(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := internal.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
		return
	}

	user, err := cfg.dbQueries.GetUserFromRefreshToken(r.Context(), refreshToken)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}

	accessToken, err := internal.MakeJWT(user.ID, cfg.jwtSecret, time.Hour)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create JWT")
		return
	}

	type response struct {
		Token string `json:"token"`
	}

	respondWithJson(w, http.StatusOK, response{
		Token: accessToken,
	})
}

func (cfg *apiConfig) revokeHandler(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := internal.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
		return
	}

	err = cfg.dbQueries.RevokeRefreshToken(r.Context(), refreshToken)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't revoke refresh token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password         string `json:"password"`
		Email            string `json:"email"`
		ExpiresInSeconds *int   `json:"expires_in_seconds"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	user, err := cfg.dbQueries.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		log.Printf("GetUserByEmail error: %s", err)
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	defaultExpiration := time.Hour
	if params.ExpiresInSeconds != nil {
		requestedExpiration := time.Duration(*params.ExpiresInSeconds) * time.Second
		if requestedExpiration < defaultExpiration {
			defaultExpiration = requestedExpiration
		}
	}

	tokenString, err := internal.MakeJWT(user.ID, cfg.jwtSecret, time.Hour)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create JWT")
		return
	}

	match, err := internal.CheckPasswordHash(params.Password, user.HashedPassword)
	if err != nil {
		log.Printf("CheckPasswordHash error: %s", err)
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	if !match {
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	refreshToken := internal.MakeRefreshToken()
	_, err = cfg.dbQueries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create refresh token")
		return
	}

	type response struct {
		User
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}

	respondWithJson(w, http.StatusOK, response{
		User: User{
			ID:          user.ID,
			CreatedAt:   user.CreatedAt,
			UpdatedAt:   user.UpdatedAt,
			Email:       user.Email,
			IsChirpyRed: user.IsChirpyRed,
		},
		Token:        tokenString,
		RefreshToken: refreshToken,
	})
}

func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	hashedPassword, err := internal.HashPassword(params.Password)
	if err != nil {
		log.Printf("HashPassword error: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't hash password")
		return
	}

	user, err := cfg.dbQueries.CreateUser(r.Context(), database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		log.Printf("CreateUser error: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't create user")
		return
	}

	respondWithJson(w, http.StatusCreated, User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	})
}

func (cfg *apiConfig) getChirpsHandler(w http.ResponseWriter, r *http.Request) {
	chirps, err := cfg.dbQueries.GetChirps(r.Context())
	if err != nil {
		log.Printf("GetChirps error: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't get chirps")
		return
	}

	responseChirps := make([]Chirp, len(chirps))
	for i, chirp := range chirps {
		responseChirps[i] = Chirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		}
	}

	respondWithJson(w, http.StatusOK, responseChirps)
}

func (cfg *apiConfig) getSingleChirpHandler(w http.ResponseWriter, r *http.Request) {
	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid chirp ID")
		return
	}

	chirp, err := cfg.dbQueries.GetSingleChirp(r.Context(), chirpID)
	if err != nil {
		log.Printf("GetChirps error: %s", err)
		respondWithError(w, 404, "Couldn't get chirps")
		return
	}

	respondWithJson(w, http.StatusOK, Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	})
}

func (cfg *apiConfig) chirpHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	tokenString, err1 := internal.GetBearerToken(r.Header)
	if err1 != nil {
		respondWithError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
		return
	}

	userID, err1 := internal.ValidateJWT(tokenString, cfg.jwtSecret)
	if err1 != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid token")
		return
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
		Body:   cleaned,
		UserID: userID,
	})
	if err != nil {
		log.Printf("CreateChirp error: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't create chirp")
		return
	}

	respondWithJson(w, http.StatusCreated, Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
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

func respondWithJson(w http.ResponseWriter, code int, payload interface{}) {
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

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorResponse struct {
		Error string `json:"error"`
	}

	respondWithJson(w, code, errorResponse{Error: msg})
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
