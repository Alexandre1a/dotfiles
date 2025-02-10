package main

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// User represents the user model in our application
type User struct {
	ID       int    `json:"id"`
	Email    string `json:"email"`
	Password string `json:"-"`    // The "-" ensures this field is never sent in JSON responses
	Role     string `json:"role"` // Either "user" or "admin"
}

// PCBuildRequest represents a customer's PC build request
type PCBuildRequest struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	Budget      float64   `json:"budget"`
	UseCase     string    `json:"use_case"`
	Preferences string    `json:"preferences"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func main() {
	// Initialize router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Public routes
	r.Group(func(r chi.Router) {
		r.Get("/", handleHome)
		r.Post("/register", handleRegister)
		r.Post("/login", handleLogin)
	})

	// Protected user routes
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Post("/build-requests", handleNewBuildRequest)
		r.Get("/build-requests", handleGetUserBuildRequests)
	})

	// Admin-only routes
	r.Group(func(r chi.Router) {
		r.Use(adminMiddleware)
		r.Get("/admin/users", handleGetAllUsers)
		r.Get("/admin/build-requests", handleGetAllBuildRequests)
		r.Put("/admin/build-requests/{id}", handleUpdateBuildStatus)
	})

	log.Fatal(http.ListenAndServe(":8080", r))
}

// Authentication middleware
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Verify JWT token here
		// Add proper JWT validation logic

		next.ServeHTTP(w, r)
	})
}

// Admin middleware - additional layer after authentication
func adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First run the auth middleware
		authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Then check if the user is an admin
			// Add logic to verify admin role from JWT claims

			next.ServeHTTP(w, r)
		})).ServeHTTP(w, r)
	})
}
