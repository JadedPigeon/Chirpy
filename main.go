package main

// Local host is http://localhost:8080

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/JadedPigeon/Chirpy/internal/auth"
	"github.com/JadedPigeon/Chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// Structs
type apiConfig struct {
	fileserverHits atomic.Int32
	DB             *database.Queries
	Platform       string
	JWTSecret      string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type userLoginResponse struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
}

type chirp struct {
	ID        uuid.UUID `json:"id"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type chirpRequest struct {
	Body string `json:"body"`
}

type userRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	//Expiration string `json:"expires_in_seconds"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		fmt.Println("Hit counted for:", r.URL.Path) // Add this
		next.ServeHTTP(w, r)
	})
}

// Handlers
func (cfg *apiConfig) fileserverHitsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)
	hits := cfg.fileserverHits.Load()
	body := fmt.Sprintf(`<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, hits)
	w.Write([]byte(body))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	if cfg.Platform != "dev" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(403)
		return
	}

	err := cfg.DB.DeleteAllUsers(r.Context())
	if err != nil {
		fmt.Printf("Error deleting users: %s", err)
		w.WriteHeader(500)
		w.Write([]byte("Internal Server Error"))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	cfg.fileserverHits.Store(0)
	hits := cfg.fileserverHits.Load()
	body := fmt.Sprintf("Hits have been reset to %d", hits)
	w.Write([]byte(body))
}

func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Something went wrong creating user"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	hashed, err := auth.HashPassword(req.Password)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Something went wrong hasing password"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	dbUser, err := cfg.DB.CreateUser(r.Context(), database.CreateUserParams{
		Email:          req.Email,
		HashedPassword: hashed,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Something went wrong creating user in the database"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	user := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(201)
	resp, _ := json.Marshal(user)
	w.Write(resp)
}

func (cfg *apiConfig) createChirpHandler(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	msg := chirpRequest{}
	err := decoder.Decode(&msg)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Invalid chirp format"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Invalid token"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	userID, err := auth.ValidateJWT(tokenString, cfg.JWTSecret)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Token not tied to a user"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	if len(msg.Body) > 140 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)

		resp := map[string]string{"error": "Chirp is too long"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	msg.Body = profanityScrubber(msg.Body)

	dbChirp, err := cfg.DB.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   msg.Body,
		UserID: userID,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Something went wrong creating chirp in the database"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	chirp := chirp{
		ID:        dbChirp.ID,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(201)
	resp, _ := json.Marshal(chirp)
	w.Write(resp)
}

func (cfg *apiConfig) getChirpsHandler(w http.ResponseWriter, r *http.Request) {
	dbchirps, err := cfg.DB.GetChirps(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Something went wrong getting chirps from the database"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	var chirps []chirp
	for _, dbChirp := range dbchirps {
		chirp := chirp{
			ID:        dbChirp.ID,
			Body:      dbChirp.Body,
			UserID:    dbChirp.UserID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
		}
		chirps = append(chirps, chirp)
	}

	chirpsjson, err := json.Marshal(chirps)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Failed to marshal chirps"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	w.Write(chirpsjson)
}

func (cfg *apiConfig) getChirpByIDHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("chirpID")
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(400)
		resp := map[string]string{"error": "Invalid chirp ID"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	dbchirp, err := cfg.DB.GetChirpByID(r.Context(), id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err == sql.ErrNoRows {
			w.WriteHeader(404)
			resp := map[string]string{"error": "Chirp not found"}
			dat, _ := json.Marshal(resp)
			w.Write(dat)
			return
		}
		w.WriteHeader(500)
		resp := map[string]string{"error": "Something went wrong getting chirp from the database"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	chirp := chirp{
		ID:        dbchirp.ID,
		Body:      dbchirp.Body,
		UserID:    dbchirp.UserID,
		CreatedAt: dbchirp.CreatedAt,
		UpdatedAt: dbchirp.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	resp, _ := json.Marshal(chirp)
	w.Write(resp)
}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Something went wrong logging in user"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	dbUser, err := cfg.DB.FindUserByEmail(r.Context(), req.Email)

	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Incorrect email or password"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	hash := dbUser.HashedPassword
	err = auth.CheckPasswordHash(req.Password, hash)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Incorrect email or password"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	expiration := time.Duration(3600) * time.Second

	token, err := auth.MakeJWT(dbUser.ID, cfg.JWTSecret, expiration)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Could not create token"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	refreshToken, err := auth.MakeRefreshToken()
	refreshExpiresAt := time.Now().Add(60 * 24 * time.Hour)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Could not create refresh token"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}
	dbRefreshToken, err := cfg.DB.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		RefreshToken: refreshToken,
		UserID:       dbUser.ID,
		ExpiresAt:    refreshExpiresAt,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Could not insert refresh token"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	user := userLoginResponse{
		ID:           dbUser.ID,
		CreatedAt:    dbUser.CreatedAt,
		UpdatedAt:    dbUser.UpdatedAt,
		Email:        dbUser.Email,
		Token:        token,
		RefreshToken: dbRefreshToken.RefreshToken,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	resp, _ := json.Marshal(user)
	w.Write(resp)
}

func (cfg *apiConfig) refreshHandler(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Invalid or missing refresh token"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	dbRefreshToken, err := cfg.DB.FindRefreshToken(r.Context(), refreshToken)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Refresh token not found"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	if dbRefreshToken.RevokedAt.Valid {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Refresh token revoked"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	// Check if expired
	if time.Now().After(dbRefreshToken.ExpiresAt) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Refresh token expired"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}
	// streak protection
	// Issue new JWT
	expiration := time.Duration(3600) * time.Second
	token, err := auth.MakeJWT(dbRefreshToken.UserID, cfg.JWTSecret, expiration)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Could not create token"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	resp := map[string]string{"token": token}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	dat, _ := json.Marshal(resp)
	w.Write(dat)
}

func (cfg *apiConfig) revokeHandler(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Invalid or missing refresh token"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	_, err = cfg.DB.RevokeRefreshToken(r.Context(), refreshToken)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(401)
		resp := map[string]string{"error": "Refresh token not found"}
		dat, _ := json.Marshal(resp)
		w.Write(dat)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	return
}

// Helper functions
func profanityScrubber(s string) string {
	badwords := []string{
		"kerfuffle",
		"sharbert",
		"fornax"}

	words := strings.Split(s, " ")
	for i, word := range words {
		for _, badword := range badwords {
			if strings.EqualFold(word, badword) {
				words[i] = "****"
				break
			}
		}
	}
	scrubbed := strings.Join(words, " ")
	return scrubbed
}

func main() {
	godotenv.Load()
	platform := os.Getenv("PLATFORM")
	secret := os.Getenv("JWT_SECRET")
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Println("Error loading db: ", err)
	}
	err = db.Ping()
	if err != nil {
		fmt.Println("Error connecting to DB: ", err)
		return
	}

	cfg := &apiConfig{}
	dbQueries := database.New(db)
	cfg.DB = dbQueries
	cfg.Platform = platform
	cfg.JWTSecret = secret

	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("")))))

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}

	mux.HandleFunc("GET /api/healthz", healthHandler)
	mux.HandleFunc("GET /admin/metrics", cfg.fileserverHitsHandler)
	mux.HandleFunc("POST /admin/reset", cfg.resetHandler)
	mux.HandleFunc("POST /api/users", cfg.createUserHandler)
	mux.HandleFunc("GET /api/chirps", cfg.getChirpsHandler)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.getChirpByIDHandler)
	mux.HandleFunc("POST /api/chirps", cfg.createChirpHandler)
	mux.HandleFunc("POST /api/login", cfg.loginHandler)
	mux.HandleFunc("POST /api/refresh", cfg.refreshHandler)
	mux.HandleFunc("POST /api/revoke", cfg.revokeHandler)

	err = srv.ListenAndServe()
	if err != nil {
		fmt.Println("Server error:", err)
	}
}
