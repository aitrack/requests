package requests

import (
	"testing"
)

func TestCrawler(t *testing.T) {
	EnableLog()

	// RegisterAgent("", &Agent{Execute: ExecuteCrawler})

	req := NewRequest()
	ExecuteCrawler(req, []string{"CX102781571AR"}, "", "", "", "")
}

func ExecuteCrawler(r *Request, trackingNoList []string, lan, postcode, dest, date string) []*TrackingItem {
	var resp *Response
	var err error

	// 1. Get csrf-token
	resp, err = r.Get("https://service.post.ch/ekp-web/ui/list", nil)
	if err != nil {
		panic(err)
	}
	csrfToken := resp.Header("X-Csrf-Token")
	if csrfToken == "" {
		return MakeBatchFatalTrackingItem(trackingNoList, 206, "$无法获取csrf-token$", resp.AsString())
	}

	// 2. Get userId
	resp, err = r.AcceptJSON().Get("https://service.post.ch/ekp-web/api/user", nil)
	if err != nil {
		panic(err)
	}
	userIdRsp := resp.AsJson()
	userId := GetMapString(userIdRsp, "userIdentifier")
	if csrfToken == "" {
		return MakeBatchFatalTrackingItem(trackingNoList, 206, "$无法获取user-id$", resp.AsString())
	}

	result := make([]*TrackingItem, 0)
	// 3. Get hash
	// 多单号查询，不应使用循环，此处是示例。
	for _, trackingNo := range trackingNoList {
		trackingItem := NewTrackingItem(trackingNo)
		result = append(result, trackingItem)

		resp, err = r.AcceptJSON().
			Header("authority", "service.post.ch").
			Header("sec-ch-ua", `"Google Chrome";v="95", "Chromium";v="95", ";Not A Brand";v="99"`).
			Header("accept-language", "de").
			Header("sec-ch-ua-mobile", "?0").
			Header("sec-ch-ua-platform", "\"Windows\"").
			Header("origin", "https://service.post.ch").
			Header("sec-fetch-site", "same-origin").
			Header("sec-fetch-mode", "cors").
			Header("sec-fetch-dest", "empty").
			Header("referer", "https://service.post.ch/ekp-web/ui/list").
			Header("x-csrf-token", csrfToken).
			PostJson("https://service.post.ch/ekp-web/api/history", map[string]string{"userId": userId}, map[string]string{"searchQuery": trackingNo})
		if err != nil {
			panic(err)
		}
		hashRsp := resp.AsJson()
		hash := GetMapString(hashRsp, "hash")
		if hash == "" {
			trackingItem.Code = 206
			trackingItem.CodeMg = "$无法获取hash$"
			trackingItem.CMess = resp.AsString()
			continue
		}

		// 4. Get identity
		resp, err = r.AcceptJSON().
			Header("authority", "service.post.ch").
			Header("sec-ch-ua", `"Google Chrome";v="95", "Chromium";v="95", ";Not A Brand";v="99"`).
			Header("accept-language", "de").
			Header("sec-ch-ua-mobile", "?0").
			Header("sec-ch-ua-platform", "\"Windows\"").
			Header("sec-fetch-site", "same-origin").
			Header("sec-fetch-mode", "cors").
			Header("sec-fetch-dest", "empty").
			Get("https://service.post.ch/ekp-web/api/history/not-included/"+hash, map[string]string{"userId": userId})
		if err != nil {
			panic(err)
		}
		identityRsp := resp.AsJsonArray()
		identity := GetMapString(identityRsp[0], "identity")
		if identity == "" {
			trackingItem.Code = 206
			trackingItem.CodeMg = "$无法获取identity$"
			trackingItem.CMess = resp.AsString()
			continue
		}

		// 5. Actually query
		resp, err = r.AcceptJSON().
			Header("authority", "service.post.ch").
			Header("sec-ch-ua", `"Google Chrome";v="95", "Chromium";v="95", ";Not A Brand";v="99"`).
			Header("accept-language", "de").
			Header("sec-ch-ua-mobile", "?0").
			Header("sec-ch-ua-platform", "\"Windows\"").
			Header("sec-fetch-site", "same-origin").
			Header("sec-fetch-mode", "cors").
			Header("sec-fetch-dest", "empty").
			Get("https://service.post.ch/ekp-web/api/shipment/id/"+identity+"/events/", nil)
		if err != nil {
			panic(err)
		}
		resultRsp := resp.AsJsonArray()

		events := make([]*TrackingEvent, 0)
		for _, resultItem := range resultRsp {
			zip := GetMapString(resultItem, "zip")
			city := GetMapString(resultItem, "city")
			// eventCode := GetMapString(resultItem, "eventCode")
			// eventCodeType := GetMapString(resultItem, "eventCodeType")
			// shipmentNumber := GetMapString(resultItem, "shipmentNumber")
			timestamp := ParseDateTime(GetMapString(resultItem, "timestamp"))

			event := TrackingEvent{
				Place: zip + " " + city,
				Date:  timestamp.Format("2002-07-05"),
			}
			events = append(events, &event)
		}

		trackingItem.Events = events
	}

	return result
}
