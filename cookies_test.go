package requests

import (
	"fmt"
	"regexp"
	"testing"
)

func TestCookies(t *testing.T) {
	EnableLog()

	r := NewRequest(nil)

	var resp *Response
	var err error

	// 1. csrf
	resp, err = r.
		Header("Connection", "keep-alive").
		Header("Pragma", "no-cache").
		Header("Cache-Control", "no-cache").
		Header("sec-ch-ua", "\"Google Chrome\";v=\"95\", \"Chromium\";v=\"95\", \";Not A Brand\";v=\"99\"").
		Header("sec-ch-ua-mobile", "?0").
		Header("sec-ch-ua-platform", "\"Windows\"").
		Header("Upgrade-Insecure-Requests", "1").
		Header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/95.0.4638.54 Safari/537.36").
		Header("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9").
		Header("Sec-Fetch-Site", "none").
		Header("Sec-Fetch-Mode", "navigate").
		Header("Sec-Fetch-User", "?1").
		Header("Sec-Fetch-Dest", "document").
		Header("Accept-Language", "zh-CN,zh;q=0.9").
		Get("https://www.asianacargo.com/tracking/viewTraceAirWaybill.do", nil)
	if err != nil {
		panic(err)
	}

	csrfSlice := regexp.MustCompile("name=\"_csrf\" value=\"(.*?)\" />").FindStringSubmatch(resp.AsString())

	if len(csrfSlice) != 2 {
		t.Errorf("expected csrf, but none got\n")
	}

	csrf := csrfSlice[1]

	payload := map[string]any{
		"prefix":    []string{"988", "988", "988", "988", "988"},
		"awbNumber": []string{"42819486", "", "", "", ""},
		"_csrf":     csrf,
	}

	resp, err = r.AcceptJSON().
		Header("Connection", "keep-alive").
		Header("Pragma", "no-cache").
		Header("Cache-Control", "no-cache").
		Header("sec-ch-ua", "\"Google Chrome\";v=\"95\", \"Chromium\";v=\"95\", \";Not A Brand\";v=\"99\"").
		Header("X-CSRF-TOKEN", csrf).
		Header("sec-ch-ua-mobile", "?0").
		Header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/95.0.4638.54 Safari/537.36").
		Header("Accept", "*/*").
		Header("X-Requested-With", "XMLHttpRequest").
		Header("sec-ch-ua-platform", "\"Windows\"").
		Header("Origin", "https://asianacargo.com").
		Header("Sec-Fetch-Site", "same-origin").
		Header("Sec-Fetch-Mode", "cors").
		Header("Sec-Fetch-Dest", "empty").
		Header("Accept-Language", "zh-CN,zh;q=0.9").
		Post("https://www.asianacargo.com/tracking/searchTraceAirWaybillResult.do", nil, payload)
	if err != nil {
		panic(err)
	}

	fmt.Println(resp.AsString())
}
