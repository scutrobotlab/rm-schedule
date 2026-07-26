package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kataras/iris/v12"
)

func TestParseTeamAbbreviations(t *testing.T) {
	data := []byte("\xEF\xBB\xBF学校名称,队伍名称,学校简称,最终简称\n" +
		"上海交通大学,交龙,上交,上交\n" +
		"中国石油大学（北京）,SPR,石大北京,SPR\n" +
		"空简称学校,Team,空校,\n" +
		",Team,空校,无学校\n" +
		"上海交通大学,交龙,上交,交大\n")

	got, err := parseTeamAbbreviations(data)
	if err != nil {
		t.Fatalf("parseTeamAbbreviations returned error: %v", err)
	}
	want := map[string]string{
		"上海交通大学":     "交大",
		"中国石油大学（北京）": "SPR",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for school, abbreviation := range want {
		if got[school] != abbreviation {
			t.Errorf("got %q for %q, want %q", got[school], school, abbreviation)
		}
	}
}

func TestTeamAbbreviationsHandler(t *testing.T) {
	app := iris.New()
	app.Get("/api/team_abbreviations", TeamAbbreviationsHandler)
	if err := app.Build(); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/team_abbreviations", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not a string map: %v", err)
	}
	if got["上海交通大学"] != "上交" {
		t.Fatalf("上海交通大学 = %q, want 上交", got["上海交通大学"])
	}
}
