package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

const CookieName = "pokernode_session"

type Claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"usr"`
	Expires  int64  `json:"exp"`
}

type Sessions struct {
	secret []byte
	ttl    time.Duration
}

func NewSessions(secret string, ttl time.Duration) (*Sessions, error) {
	if len(secret) < 32 {
		return nil, errors.New("POKERNODE_SESSION_SECRET must be at least 32 characters")
	}
	return &Sessions{secret: []byte(secret), ttl: ttl}, nil
}

func (s *Sessions) Issue(userID int64, username string) (string, time.Time, error) {
	expires := time.Now().Add(s.ttl)
	payload, err := json.Marshal(Claims{UserID: userID, Username: username, Expires: expires.Unix()})
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + s.sign(encoded), expires, nil
}

func (s *Sessions) Parse(value string) (Claims, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(s.sign(parts[0]))) {
		return Claims{}, errors.New("invalid session")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, errors.New("invalid session")
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil || claims.UserID <= 0 || claims.Username == "" {
		return Claims{}, errors.New("invalid session")
	}
	if time.Now().Unix() >= claims.Expires {
		return Claims{}, errors.New("session expired")
	}
	return claims, nil
}

func (s *Sessions) sign(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func CacheKey(userID int64) string {
	return strconv.FormatInt(userID, 10)
}
