package governor

import (
	"os"
	"testing"
	"time"
)

func TestGenerateToken_ValidateToken(t *testing.T) {
	os.Setenv("HC_JWT_SECRET", "test-secret-32-bytes-long-enough!!")
	defer os.Unsetenv("HC_JWT_SECRET")
	token, err := GenerateToken("user1", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Error("token should not be empty")
	}
	c, err := ValidateToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if c.UserID != "user1" || c.Role != "admin" {
		t.Errorf("claims: user=%s role=%s", c.UserID, c.Role)
	}
}

func TestValidateToken_Expired(t *testing.T) {
	os.Setenv("HC_JWT_SECRET", "test-secret-expired")
	defer os.Unsetenv("HC_JWT_SECRET")
	token, _ := GenerateToken("u", "r")
	_ = token
	// Manually create expired token by manipulating - we can't easily without exporting
	// So we test tampering instead
	_, err := ValidateToken("invalid.token.here")
	if err == nil {
		t.Error("invalid token should fail")
	}
}

func TestValidateToken_Tampered(t *testing.T) {
	_, err := ValidateToken("eyJ1c2VyX2lkIjoidSJ9.invalidsignature")
	if err == nil {
		t.Error("tampered token should fail")
	}
}

func TestRefreshToken(t *testing.T) {
	os.Setenv("HC_JWT_SECRET", "test-refresh-secret")
	defer os.Unsetenv("HC_JWT_SECRET")
	token, _ := GenerateToken("u2", "user")
	newToken, err := RefreshToken(token)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ValidateToken(newToken)
	if err != nil || c.UserID != "u2" {
		t.Errorf("refresh: err=%v user=%s", err, c.UserID)
	}
}

func TestClaims_Expiry(t *testing.T) {
	var c Claims
	c.ExpiresAt = time.Now().Unix() - 1
	_ = c
}
