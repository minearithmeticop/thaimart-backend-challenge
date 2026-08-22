package config

import (
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if c.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q", c.HTTPAddr)
	}
	if c.GRPCAddr != ":50051" {
		t.Errorf("GRPCAddr = %q", c.GRPCAddr)
	}
	if c.MongoURI != "mongodb://localhost:27017" {
		t.Errorf("MongoURI = %q", c.MongoURI)
	}
	if c.MongoDB != "thaimart" {
		t.Errorf("MongoDB = %q", c.MongoDB)
	}
	if c.JWTTTL != time.Hour {
		t.Errorf("JWTTTL = %s", c.JWTTTL)
	}
	if c.BcryptCost != 10 {
		t.Errorf("BcryptCost = %d", c.BcryptCost)
	}
	if c.LogFormat != "text" {
		t.Errorf("LogFormat = %q", c.LogFormat)
	}
	if c.ReportEvery != 10*time.Second {
		t.Errorf("ReportEvery = %s", c.ReportEvery)
	}
	if c.JWTSecret == "" {
		t.Error("expected an ephemeral secret to be generated when JWT_SECRET is unset")
	}
}

func TestOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9000")
	t.Setenv("GRPC_ADDR", ":60051")
	t.Setenv("MONGO_URI", "mongodb://mongo:27017")
	t.Setenv("MONGO_DB", "other")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("JWT_TTL", "1h30m")
	t.Setenv("BCRYPT_COST", "12")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("REPORT_EVERY", "45s")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if c.HTTPAddr != ":9000" || c.GRPCAddr != ":60051" || c.MongoURI != "mongodb://mongo:27017" || c.MongoDB != "other" {
		t.Errorf("string overrides mismatch: %+v", c)
	}
	if c.JWTSecret != "0123456789abcdef0123456789abcdef" {
		t.Errorf("JWTSecret = %q", c.JWTSecret)
	}
	if c.JWTTTL != 90*time.Minute {
		t.Errorf("JWTTTL = %s", c.JWTTTL)
	}
	if c.BcryptCost != 12 {
		t.Errorf("BcryptCost = %d", c.BcryptCost)
	}
	if c.LogFormat != "json" {
		t.Errorf("LogFormat = %q", c.LogFormat)
	}
	if c.ReportEvery != 45*time.Second {
		t.Errorf("ReportEvery = %s", c.ReportEvery)
	}
}

func TestInvalidValuesFailFast(t *testing.T) {
	cases := []struct{ key, value string }{
		{"JWT_TTL", "not-a-duration"},
		{"JWT_TTL", "0s"},
		{"JWT_TTL", "-5m"},
		{"BCRYPT_COST", "abc"},
		{"BCRYPT_COST", "3"},
		{"BCRYPT_COST", "16"},
		{"LOG_FORMAT", "xml"},
		{"REPORT_EVERY", "not-a-duration"},
		{"REPORT_EVERY", "0s"},
		{"REPORT_EVERY", "-5m"},
	}

	for _, tc := range cases {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			for _, k := range []string{"JWT_TTL", "BCRYPT_COST", "LOG_FORMAT", "REPORT_EVERY"} {
				t.Setenv(k, "")
			}
			t.Setenv(tc.key, tc.value)

			if _, err := Load(); err == nil {
				t.Fatalf("expected a startup error for %s=%q", tc.key, tc.value)
			}
		})
	}
}
