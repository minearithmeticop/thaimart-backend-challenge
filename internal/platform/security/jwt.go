package security

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

var _ app.TokenManager = (*JWTManager)(nil)

// JWTManager implements app.TokenManager. Tokens are signed with
// HMAC-SHA256 (HS256) using a shared secret. Verification pins the
// algorithm to HS256 so an attacker cannot swap in "none" or an RS*
// family header to bypass the signature check (the classic
// alg-confusion attack).
type JWTManager struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

func NewJWTManager(secret string, ttl time.Duration) *JWTManager {
	return &JWTManager{
		secret: []byte(secret),
		ttl:    ttl,
		issuer: "thaimart-user-api",
	}
}

func (m *JWTManager) Generate(userID, email string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.ttl)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"iss":   m.issuer,
		"iat":   now.Unix(),
		"exp":   expiresAt.Unix(),
	})
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("signing token: %w", err)
	}
	return signed, expiresAt, nil
}

func (m *JWTManager) Verify(tokenString string) (app.Claims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		// Pin the algorithm family: only HS256 is ever accepted.
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return app.Claims{}, domain.ErrUnauthorized
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return app.Claims{}, domain.ErrUnauthorized
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return app.Claims{}, domain.ErrUnauthorized
	}
	email, _ := claims["email"].(string)

	var expiresAt time.Time
	if exp, ok := claims["exp"].(float64); ok {
		expiresAt = time.Unix(int64(exp), 0)
	}
	return app.Claims{UserID: sub, Email: email, ExpiresAt: expiresAt}, nil
}
