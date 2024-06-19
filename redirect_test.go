package requests

import (
	"testing"
)

func TestRedirect(t *testing.T) {
	EnableLog()

	r := NewRequest(nil)

	resp, err := r.Header("Connection", "keep-alive").
		Header("sec-ch-ua", "\"Google Chrome\";v=\"95\", \"Chromium\";v=\"95\", \";Not A Brand\";v=\"99\"").
		Header("sec-ch-ua-mobile", "?0").
		Header("sec-ch-ua-platform", "\"Windows\"").
		Header("Upgrade-Insecure-Requests", "1").
		Header("DNT", "1").
		Header("Sec-Fetch-Site", "same-site").
		Header("Sec-Fetch-Mode", "navigate").
		Header("Sec-Fetch-User", "?1").
		Header("Sec-Fetch-Dest", "document").
		Header("Referer", "https://www.dpd.co.uk/").
		Header("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8").
		Header("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9").
		Get("https://apis.track.dpd.co.uk/v1/track", map[string]string{"postcode": "", "parcel": "15501275614138"})
	if err != nil {
		panic(err)
	}

	u1 := resp.URL()
	r1 := "https://track.dpd.co.uk/parcels/15501275614138*19647"

	if u1 != r1 {
		t.Errorf("expected %s, but %s got\n", r1, u1)
	}
}
