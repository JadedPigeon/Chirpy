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

	"github.com/JadedPigeon/Chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	DB             *database.Queries
	Platform       string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
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

func (cfg *apiConfig) validateChirpHandler(w http.ResponseWriter, r *http.Request) {
	type chirp struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	msg := chirp{}
	err := decoder.Decode(&msg)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		resp := map[string]string{"error": "Something went wrong"}
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	resp := map[string]string{"cleaned_body": profanityScrubber(msg.Body)}
	dat, _ := json.Marshal(resp)
	w.Write(dat)
}

func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
	type userRequest struct {
		Email string `json:"email"`
	}
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

	dbUser, err := cfg.DB.CreateUser(r.Context(), req.Email)

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
	mux.HandleFunc("POST /api/validate_chirp", cfg.validateChirpHandler)
	mux.HandleFunc("POST /api/users", cfg.createUserHandler)

	err = srv.ListenAndServe()
	if err != nil {
		fmt.Println("Server error:", err)
	}
}
