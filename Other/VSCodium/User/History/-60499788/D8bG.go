package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/bcrypt"
)

// User represents the user model in our application
type User struct {
	ID       int    `json:"id"`
	Email    string `json:"email"`
	Password string `json:"-"`
	Role     string `json:"role"`
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

// Global database connection
var db *sql.DB

func main() {
	// Initialize database connection
	var err error
	db, err = sql.Open("mysql", "myuser:mypassword@tcp(mysql:3306)/mydatabase?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

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

	log.Printf("Server starting on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// Handler Functions

func handleHome(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Welcome to Custom PC Builder API"))
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Error processing registration", http.StatusInternalServerError)
		return
	}

	// Insert user into database
	result, err := db.Exec(
		"INSERT INTO users (email, password_hash, role) VALUES (?, ?, 'user')",
		user.Email, hashedPassword,
	)
	if err != nil {
		http.Error(w, "Error creating user", http.StatusInternalServerError)
		return
	}

	userID, _ := result.LastInsertId()
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      userID,
		"message": "User registered successfully",
	})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var loginData struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&loginData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Retrieve user from database
	var user User
	var hashedPassword string
	err := db.QueryRow(
		"SELECT id, email, password_hash, role FROM users WHERE email = ?",
		loginData.Email,
	).Scan(&user.ID, &user.Email, &hashedPassword, &user.Role)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(loginData.Password)); err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Generate JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"role":    user.Role,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		http.Error(w, "Error generating token", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"token": tokenString,
	})
}

func handleNewBuildRequest(w http.ResponseWriter, r *http.Request) {
	var buildRequest PCBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&buildRequest); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get user ID from context (set by auth middleware)
	userID := r.Context().Value("user_id").(int)
	buildRequest.UserID = userID

	result, err := db.Exec(
		"INSERT INTO pc_build_requests (user_id, budget, use_case, preferences, status) VALUES (?, ?, ?, ?, 'pending')",
		buildRequest.UserID, buildRequest.Budget, buildRequest.UseCase, buildRequest.Preferences,
	)
	if err != nil {
		http.Error(w, "Error creating build request", http.StatusInternalServerError)
		return
	}

	buildID, _ := result.LastInsertId()
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      buildID,
		"message": "Build request created successfully",
	})
}

func handleGetUserBuildRequests(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)

	rows, err := db.Query(
		"SELECT id, user_id, budget, use_case, preferences, status, created_at FROM pc_build_requests WHERE user_id = ?",
		userID,
	)
	if err != nil {
		http.Error(w, "Error retrieving build requests", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var requests []PCBuildRequest
	for rows.Next() {
		var req PCBuildRequest
		err := rows.Scan(&req.ID, &req.UserID, &req.Budget, &req.UseCase, &req.Preferences, &req.Status, &req.CreatedAt)
		if err != nil {
			continue
		}
		requests = append(requests, req)
	}

	json.NewEncoder(w).Encode(requests)
}

func handleGetAllUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, email, role FROM users")
	if err != nil {
		http.Error(w, "Error retrieving users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		err := rows.Scan(&user.ID, &user.Email, &user.Role)
		if err != nil {
			continue
		}
		users = append(users, user)
	}

	json.NewEncoder(w).Encode(users)
}

func handleGetAllBuildRequests(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(
		"SELECT id, user_id, budget, use_case, preferences, status, created_at FROM pc_build_requests",
	)
	if err != nil {
		http.Error(w, "Error retrieving build requests", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var requests []PCBuildRequest
	for rows.Next() {
		var req PCBuildRequest
		err := rows.Scan(&req.ID, &req.UserID, &req.Budget, &req.UseCase, &req.Preferences, &req.Status, &req.CreatedAt)
		if err != nil {
			continue
		}
		requests = append(requests, req)
	}

	json.NewEncoder(w).Encode(requests)
}

func handleUpdateBuildStatus(w http.ResponseWriter, r *http.Request) {
	buildID := chi.URLParam(r, "id")
	var updateData struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	result, err := db.Exec(
		"UPDATE pc_build_requests SET status = ? WHERE id = ?",
		updateData.Status, buildID,
	)
	if err != nil {
		http.Error(w, "Error updating build request", http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Build request not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Build request updated successfully",
	})
}
