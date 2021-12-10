package requests

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func GetMapString(m map[string]interface{}, key string) string {
	s := m[key]
	if s == nil {
		return ""
	} else {
		return fmt.Sprintf("%s", s)
	}
}

func GetMapInt64(m map[string]interface{}, key string) int64 {
	s := m[key]
	if s == nil {
		return 0
	} else {
		if r, err := strconv.ParseInt(fmt.Sprintf("%s", s), 10, 64); err != nil {
			return 0
		} else {
			return r
		}
	}
}

func ParseDateTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if r, err := time.Parse("2006-01-02T15:04:05-07:00", s); err != nil {
		return time.Time{}
	} else {
		return r.UTC()
	}
}

func FormatDateTime(t time.Time) string {
	t = t.UTC()
	if t.IsZero() {
		return ""
	} else {
		return t.Format("2006-01-02 15:04:05")
	}
}

// 缩减空格。
// 将所有连续的空格减少只有一个空格。
// s 原始字符串。
// 返回缩减后的字符串。
func ReduceWhitespaces(s string) string {
	buf := make([]rune, 0, len(s))
	isWhitespace := false
	for _, ch := range s {
		if unicode.IsSpace(ch) {
			if !isWhitespace {
				buf = append(buf, ch)
			}
			isWhitespace = true
		} else {
			buf = append(buf, ch)
			isWhitespace = false
		}
	}
	return string(buf)
}

func MatchOddityCode(value, pattern string) bool {
	vv := strings.Split(value, ".")
	pv := strings.Split(pattern, ".")

	for i, pp := range pv {
		if pp == "*" {
			continue
		} else {
			if i >= len(vv) {
				return false
			} else if pp != vv[i] {
				return false
			}
		}
	}

	return true
}

func MakeBatchFatalTrackingItem(trackignNoList []string, code int, message, raw string) []*TrackingItem {
	result := make([]*TrackingItem, 0)

	for _, trackingNo := range trackignNoList {
		item := NewTrackingItem(trackingNo)
		item.Code = code
		item.CodeMg = message
		item.CMess = raw

		result = append(result, item)
	}

	return result
}

// FillEmptyClock 填充空白的时间部分。
func FillEmptyClock(t *time.Time, sHour, sMin, sSec *int) {
	year, month, day := t.Date()
	hour, min, sec := t.Clock()

	if hour == 0 && min == 0 && sec == 0 {
		// 缺失时间部分，需要补充。

		*sSec++
		if *sSec > 59 {
			*sSec = 0
			*sMin++
		}
		if *sMin > 59 {
			*sMin = 0
			*sHour++
		}
		if *sHour > 23 {
			*sHour = 0
		}

		*t = time.Date(year, month, day, *sHour, *sMin, *sSec, 0, t.Location())
	}
}

func VerifyWithMd5(sign string, args ...string) bool {
	plainText := strings.Join(args, "")
	md5Bytes := md5.Sum([]byte(plainText))
	return hex.EncodeToString(md5Bytes[:]) == sign
}
