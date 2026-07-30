package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kataras/iris/v12"
)

func TestParseTeamAbbreviations(t *testing.T) {
	data := []byte("\xEF\xBB\xBF学校名称,队伍名称,简称4字,简称2字\n" +
		"上海交通大学,交龙,上海交大,上交\n" +
		"中国石油大学（华东）,RPS,石大华东,RPS\n" +
		"空简称学校,Team,空校,\n" +
		",Team,空校,无学校\n" +
		"上海交通大学,交龙,上海交大,交大\n")

	got, err := parseTeamAbbreviations(data)
	if err != nil {
		t.Fatalf("parseTeamAbbreviations returned error: %v", err)
	}
	want := map[string]teamAbbreviation{
		"上海交通大学": {
			Abbreviation4: "上海交大",
			Abbreviation2: "交大",
		},
		"中国石油大学（华东）": {
			Abbreviation4: "石大华东",
			Abbreviation2: "RPS",
		},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for school, abbreviation := range want {
		if got[school] != abbreviation {
			t.Errorf("got %#v for %q, want %#v", got[school], school, abbreviation)
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
	if got := rec.Header().Get("Cache-Control"); got != longLivedStaticCacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, longLivedStaticCacheControl)
	}
	var got map[string]teamAbbreviation
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not an abbreviation map: %v", err)
	}
	want := teamAbbreviation{Abbreviation4: "上海交大", Abbreviation2: "上交"}
	if got["上海交通大学"] != want {
		t.Fatalf("上海交通大学 = %#v, want %#v", got["上海交通大学"], want)
	}
}
