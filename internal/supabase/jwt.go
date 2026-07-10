package supabase

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims is the subset of a Supabase access token JWT we care about.
type Claims struct {
	UserID uuid.UUID
	Email  string
	Expiry time.Time
}

// VerifyAccessToken locally verifies a Supabase-issued access token JWT
// (HS256, signed with the project's JWT secret) without a round-trip to
// Supabase. jwt.Parse validates the exp/nbf claims automatically.
func VerifyAccessToken(tokenString, jwtSecret string) (*Claims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(jwtSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("verify access token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid access token")
	}

	sub, _ := claims["sub"].(string)
	id, err := uuid.Parse(sub)
	if err != nil {
		return nil, fmt.Errorf("invalid subject claim: %w", err)
	}

	email, _ := claims["email"].(string)

	var expiry time.Time
	if exp, ok := claims["exp"].(float64); ok {
		expiry = time.Unix(int64(exp), 0)
	}

	return &Claims{UserID: id, Email: email, Expiry: expiry}, nil
}
