package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// handleLogin validates user credentials and issues a JWT via a secure cookie.
func handleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var userID, orgID uuid.UUID
	var hashedPassword string
	var isCustomerAdmin bool

	// CORRECTED QUERY: Selects `id`, `organization_id`, `hashed_password`, and `is_customer_admin`
	// from the `users` table, aligning with the init.sql schema.
	query := `SELECT id, organization_id, hashed_password, is_customer_admin FROM users WHERE email = $1 AND is_system_admin = FALSE`
	err := dbpool.QueryRow(context.Background(), query, req.Email).Scan(&userID, &orgID, &hashedPassword, &isCustomerAdmin)
	if err != nil {
		// This handles both "user not found" and other potential DB errors.
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Compare the provided password with the stored hash.
	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password))
	if err != nil {
		// Password does not match.
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Create JWT claims for the authenticated user.
	expirationTime := time.Now().Add(24 * time.Hour) // Token is valid for 24 hours
	claims := &Claims{
		UserID:          userID.String(),
		OrganizationID:  orgID.String(),
		IsCustomerAdmin: isCustomerAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) == 0 {
		zlog.Error().Msg("JWT_SECRET environment variable not set")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server configuration error"})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to sign JWT token")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not process login"})
		return
	}

	// Set the token in a secure, HttpOnly cookie.
	c.SetCookie("token", tokenString, int(expirationTime.Sub(time.Now()).Seconds()), "/api", "", true, true)
	c.JSON(http.StatusOK, gin.H{"message": "Successfully logged in"})
}

// handleLogout clears the authentication cookie, effectively logging the user out.
func handleLogout(c *gin.Context) {
	// To clear a cookie, we set it again with a MaxAge of -1.
	// The path and domain must match the original cookie.
	c.SetCookie("token", "", -1, "/api", "", true, true)
	c.JSON(http.StatusOK, gin.H{"message": "Successfully logged out"})
}
