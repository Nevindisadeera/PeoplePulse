package auth

import (
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "peoplepulse"

type Claims struct {
	Org  string `json:"org"`
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type TokenMaker struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenMaker(secret string, ttl time.Duration) *TokenMaker {
	return &TokenMaker{secret: []byte(secret), ttl: ttl}
}

func (m *TokenMaker) Issue(userID, orgID, roleID string) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(m.ttl)
	claims := Claims{Org: orgID, Role: roleID, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: issuer, Subject: userID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	return tok, exp, err
}

func (m *TokenMaker) Parse(token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// Limiter is a small in-memory fixed-window limiter for sign-in attempts.
// It is per process; a shared store is needed if the API runs as several instances.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*bucket
}

type bucket struct {
	count int
	reset time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, hits: map[string]*bucket{}}
}

// Allow records one attempt for key and reports whether it is within the limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.hits) > 10000 {
		for k, b := range l.hits {
			if now.After(b.reset) {
				delete(l.hits, k)
			}
		}
	}
	b, ok := l.hits[key]
	if !ok || now.After(b.reset) {
		l.hits[key] = &bucket{count: 1, reset: now.Add(l.window)}
		return true
	}
	b.count++
	return b.count <= l.max
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}
