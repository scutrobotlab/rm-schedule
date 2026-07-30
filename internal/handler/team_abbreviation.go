package handler

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/static"
)

const (
	schoolNameHeader    = "学校名称"
	abbreviation4Header = "简称4字"
	abbreviation2Header = "简称2字"
)

type teamAbbreviation struct {
	Abbreviation4 string `json:"abbreviation4"`
	Abbreviation2 string `json:"abbreviation2"`
}

func parseTeamAbbreviations(data []byte) (map[string]teamAbbreviation, error) {
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})))
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	schoolIndex := -1
	abbreviation4Index := -1
	abbreviation2Index := -1
	for i, value := range header {
		switch strings.TrimSpace(value) {
		case schoolNameHeader:
			schoolIndex = i
		case abbreviation4Header:
			abbreviation4Index = i
		case abbreviation2Header:
			abbreviation2Index = i
		}
	}
	if schoolIndex < 0 || abbreviation4Index < 0 || abbreviation2Index < 0 {
		return nil, fmt.Errorf(
			"required headers %q, %q and %q not found",
			schoolNameHeader,
			abbreviation4Header,
			abbreviation2Header,
		)
	}

	result := make(map[string]teamAbbreviation)
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read record: %w", readErr)
		}
		if schoolIndex >= len(record) ||
			abbreviation4Index >= len(record) ||
			abbreviation2Index >= len(record) {
			continue
		}
		school := strings.TrimSpace(record[schoolIndex])
		abbreviation4 := strings.TrimSpace(record[abbreviation4Index])
		abbreviation2 := strings.TrimSpace(record[abbreviation2Index])
		if school == "" || abbreviation4 == "" || abbreviation2 == "" {
			continue
		}
		result[school] = teamAbbreviation{
			Abbreviation4: abbreviation4,
			Abbreviation2: abbreviation2,
		}
	}
	return result, nil
}

func TeamAbbreviationsHandler(c iris.Context) {
	abbreviations, err := parseTeamAbbreviations(static.TeamAbbreviationBytes)
	if err != nil {
		c.StatusCode(iris.StatusInternalServerError)
		c.JSON(iris.Map{"code": -1, "msg": "Failed to parse team abbreviations"})
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(abbreviations)
}
