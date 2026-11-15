package auth

import (
	"testing"
	"time"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse battery", hash) {
		t.Error("correct password was rejected")
	}
	if VerifyPassword("wrong password here", hash) {
		t.Error("wrong password was accepted")
	}
	if VerifyPassword("anything", "not-a-hash") {
		t.Error("malformed hash was accepted")
	}
	other, _ := HashPassword("correct horse battery")
	if other == hash {
		t.Error("two hashes of the same password are identical; salt is not random")
	}
}

func TestValidatePassword(t *testing.T) {
	for pw, ok := range map[string]bool{
		"short":                 false,
		"password1234":          false,
		"PASSWORD1234":          false,
		"a-long-unique-phrase!": true,
	} {
		if got := ValidatePassword(pw) == nil; got != ok {
			t.Errorf("ValidatePassword(%q) ok = %v, want %v", pw, got, ok)
		}
	}
}

func TestTokenRoundTripAndExpiry(t *testing.T) {
	m := NewTokenMaker("0123456789abcdef0123456789abcdef", time.Minute)
	tok, _, err := m.Issue("user1", "org1", "role1")
	if err != nil {
		t.Fatal(err)
	}
	c, err := m.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "user1" || c.Org != "org1" || c.Role != "role1" {
		t.Errorf("unexpected claims: %+v", c)
	}
	if _, err := NewTokenMaker("another-secret-another-secret-xx", time.Minute).Parse(tok); err == nil {
		t.Error("token signed with a different secret was accepted")
	}
	expired, _, _ := NewTokenMaker("0123456789abcdef0123456789abcdef", -time.Minute).Issue("u", "o", "r")
	if _, err := m.Parse(expired); err == nil {
		t.Error("expired token was accepted")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	if !l.Allow("k") || !l.Allow("k") {
		t.Fatal("first two attempts should pass")
	}
	if l.Allow("k") {
		t.Error("third attempt should be blocked")
	}
	if !l.Allow("other") {
		t.Error("a different key should not be blocked")
	}
	l.Reset("k")
	if !l.Allow("k") {
		t.Error("attempt after reset should pass")
	}
}
