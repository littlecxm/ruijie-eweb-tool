package utils

import (
	"time"
)

const (
	WCTsFormat = "060102150405"
	CSTimeZone = -8 * time.Hour
)

func GetWCTimestamp(t time.Time) string {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := t.In(loc)
	return now.Format(WCTsFormat)
}

func ParseWCTimestamp(wcTs string) (*time.Time, error) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	date, err := time.Parse(WCTsFormat, wcTs)
	if err != nil {
		return nil, err
	}
	date = date.Add(CSTimeZone).In(loc)
	return &date, nil
}
