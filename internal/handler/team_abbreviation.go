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
	schoolNameHeader        = "学校名称"
	finalAbbreviationHeader = "最终简称"
)

func parseTeamAbbreviations(data []byte) (map[string]string, error) {
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})))
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	schoolIndex := -1
	abbreviationIndex := -1
	for i, value := range header {
		switch strings.TrimSpace(value) {
		case schoolNameHeader:
			schoolIndex = i
		case finalAbbreviationHeader:
			abbreviationIndex = i
		}
	}
	if schoolIndex < 0 || abbreviationIndex < 0 {
		return nil, fmt.Errorf("required headers %q and %q not found", schoolNameHeader, finalAbbreviationHeader)
	}

	result := make(map[string]string)
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read record: %w", readErr)
		}
		if schoolIndex >= len(record) || abbreviationIndex >= len(record) {
			continue
		}
		school := strings.TrimSpace(record[schoolIndex])
		abbreviation := strings.TrimSpace(record[abbreviationIndex])
		if school == "" || abbreviation == "" {
			continue
		}
		result[school] = abbreviation
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
	c.Header("Cache-Control", "public, max-age=86400")
	c.JSON(abbreviations)
}
