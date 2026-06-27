package jwtutil

import "github.com/golang-jwt/jwt/v5"

// Claims represents the custom JWT claims used by GoBox.
type Claims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
	Name  string `json:"name"`
	SID   string `json:"sid"`
}
