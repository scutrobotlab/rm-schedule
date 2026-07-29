package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			if gotCacheControl := rec.Header().Get("Cache-Control"); gotCacheControl != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", gotCacheControl)
			}
		})
	}
}
