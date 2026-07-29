package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kataras/iris/v12"
)

func TestConfigHandler(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want bool
	}{
		{name: "unset defaults to production", want: false},
		{name: "true enables test environment", env: "true", want: true},
		{name: "one enables test environment", env: "1", want: true},
		{name: "invalid value defaults to production", env: "testing", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envIsTestEnvironment, tt.env)

			app := iris.New()
			app.Get("/api/config", ConfigHandler)
			if err := app.Build(); err != nil {
				t.Fatal(err)
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
			app.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}
			var got globalConfig
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("response is not global config: %v", err)
			}
			if got.IsTestEnvironment != tt.want {
				t.Fatalf("isTestEnvironment = %t, want %t", got.IsTestEnvironment, tt.want)
			}
			if got.MobileBracketEnabled {
				t.Fatal("mobileBracketEnabled = true, want false when rollout is unset")
			}
			if gotCacheControl := rec.Header().Get("Cache-Control"); gotCacheControl != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", gotCacheControl)
			}
		})
	}
}

func TestBracketCookieRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	value := encodeBracketCookie(2374, secret)

	bucket, ok := decodeBracketCookie(value, secret)
	if !ok || bucket != 2374 {
		t.Fatalf("decodeBracketCookie() = (%d, %t), want (2374, true)", bucket, ok)
	}

	for _, invalid := range []string{
		"",
		"2374",
		"2374.bad",
		"-1." + strings.Repeat("0", 64),
		"10000." + strings.Repeat("0", 64),
		encodeBracketCookie(2374, []byte("other-secret")),
	} {
		if _, valid := decodeBracketCookie(invalid, secret); valid {
			t.Fatalf("decodeBracketCookie(%q) unexpectedly valid", invalid)
		}
	}
}

func TestConfigHandlerMobileBracketRollout(t *testing.T) {
	originalGenerator := generateBracketBucket
	t.Cleanup(func() { generateBracketBucket = originalGenerator })
	generateBracketBucket = func() (int, error) { return 2374, nil }
	t.Setenv(envExperimentCookieSecret, "test-secret")

	tests := []struct {
		rollout string
		want    bool
	}{
		{rollout: "0", want: false},
		{rollout: "20", want: false},
		{rollout: "23.74", want: false},
		{rollout: "23.75", want: true},
		{rollout: "30", want: true},
		{rollout: "100", want: true},
		{rollout: "invalid", want: false},
		{rollout: "100.01", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.rollout, func(t *testing.T) {
			t.Setenv(envMobileBracketRollout, tt.rollout)
			app := iris.New()
			app.Get("/api/config", ConfigHandler)
			if err := app.Build(); err != nil {
				t.Fatal(err)
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
			app.ServeHTTP(rec, req)

			var got globalConfig
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.MobileBracketEnabled != tt.want {
				t.Fatalf("mobileBracketEnabled = %t, want %t", got.MobileBracketEnabled, tt.want)
			}
			if rec.Header().Get("Set-Cookie") == "" {
				t.Fatal("first request did not set bracket bucket cookie")
			}
		})
	}
}

func TestConfigHandlerReusesBracketCookie(t *testing.T) {
	t.Setenv(envMobileBracketRollout, "30")
	t.Setenv(envExperimentCookieSecret, "test-secret")
	originalGenerator := generateBracketBucket
	t.Cleanup(func() { generateBracketBucket = originalGenerator })
	generateBracketBucket = func() (int, error) {
		t.Fatal("valid cookie should not generate a new bucket")
		return 0, nil
	}

	app := iris.New()
	app.Get("/api/config", ConfigHandler)
	if err := app.Build(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(&http.Cookie{
		Name:  bracketUICookieName,
		Value: encodeBracketCookie(2374, []byte("test-secret")),
	})
	app.ServeHTTP(rec, req)

	var got globalConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.MobileBracketEnabled {
		t.Fatal("mobileBracketEnabled = false, want true")
	}
	if rec.Header().Get("Set-Cookie") != "" {
		t.Fatal("valid cookie was unexpectedly replaced")
	}
}

func TestConfigHandlerBucketFailureFallsBackToControl(t *testing.T) {
	t.Setenv(envMobileBracketRollout, "100")
	t.Setenv(envExperimentCookieSecret, "test-secret")
	originalGenerator := generateBracketBucket
	t.Cleanup(func() { generateBracketBucket = originalGenerator })
	generateBracketBucket = func() (int, error) { return 0, errors.New("entropy unavailable") }

	app := iris.New()
	app.Get("/api/config", ConfigHandler)
	if err := app.Build(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	app.ServeHTTP(rec, req)

	var got globalConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.MobileBracketEnabled {
		t.Fatal("mobileBracketEnabled = true, want false")
	}
	if rec.Header().Get("Set-Cookie") != "" {
		t.Fatal("bucket generation failure should not set a cookie")
	}
}
