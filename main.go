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

	"github.com/JadedPigeon/Chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	DB             *database.Queries
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		fmt.Println("Hit counted for:", r.URL.Path) // Add this
		next.ServeHTTP(w, r)
	})
}

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

func (cfg *apiConfig) fileserverHitsResetHandler(w http.ResponseWriter, r *http.Request) {
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
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Println("Error loading db: ", err)
	}

	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	cfg := &apiConfig{}
	dbQueries := database.New(db)
	cfg.DB = dbQueries
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("")))))

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}

	mux.HandleFunc("GET /api/healthz", healthHandler)
	mux.HandleFunc("GET /admin/metrics", cfg.fileserverHitsHandler)
	mux.HandleFunc("POST /admin/reset", cfg.fileserverHitsResetHandler)
	mux.HandleFunc("POST /api/validate_chirp", cfg.validateChirpHandler)

	err = srv.ListenAndServe()
	if err != nil {
		fmt.Println("Server error:", err)
	}
}
