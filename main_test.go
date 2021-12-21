package requests

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

func init() {
	RegisterAgent("yamato", &Agent{MaxOrdersPerPage: 3, Execute: yamatoHandler})
}

func TestAgent(t *testing.T) {
	EnableLog()

	buf := bytes.NewBuffer([]byte{})
	rw := NewMockResponseWriter(buf)
	rq := NewMockRequest(map[string]string{"carriercode": "yamato", "nums": "481130000595,481130000632,481130000643,481130000610,481130000621,481130000654,4811300005a3,481130000573"})
	Run(rw, rq)

	fmt.Printf("%s\n", buf.String())
}

func yamatoHandler(r *Request, trackingNoList []string, lan, postcode, dest, date string) []*TrackingItem {
	var resp *Response
	var err error

	result := make([]*TrackingItem, 0)

	payload := map[string]string{
		"mypagesession": "",
		"backaddress":   "",
		"backrequest":   "get",
		"category":      "0",
	}

	for i := 1; i <= 10; i++ {
		trackingNo := ""
		if i <= len(trackingNoList) {
			trackingNo = trackingNoList[i-1]
		}
		payload[fmt.Sprintf("number%02d", i)] = trackingNo
	}

	resp, err = r.AcceptHTML().
		Post("https://toi.kuronekoyamato.co.jp/cgi-bin/tneko", nil, payload)
	if err != nil {
		panic(err)
	}

	if doc := resp.AsHtml(""); doc == nil {
		// TODO: 全部设置为错误。
		for _, trackingNo := range trackingNoList {
			result = append(result, &TrackingItem{
				TrackingNo: trackingNo,
				Code:       206,
				CodeMg:     "$解析页面出错$",
			})
		}
	} else {
		for i := 0; i < len(trackingNoList); i++ {
			trackingItem := NewTrackingItem(trackingNoList[i])

			// 获取anchor节点的ID，然后找到这个节点。
			divId := fmt.Sprintf("AA%d00", i)
			wrapperDiv := htmlquery.FindOne(doc, `//div[@id="`+divId+`"]`).NextSibling

			// 跳过空白文本节点，获取下一个兄弟div节点。此兄弟节点就是包含了查询结果的div。
			for wrapperDiv != nil && wrapperDiv.Type == html.TextNode {
				// TODO: 检查下一个节点的node name。
				wrapperDiv = wrapperDiv.NextSibling
			}
			if wrapperDiv == nil {
				trackingItem.Code = 206
				trackingItem.CodeMg = "$解析页面出错$"
			} else {
				finds := htmlquery.Find(wrapperDiv, "//div[@class='tracking-invoice-block-detail']/ol/li")

				sSec := 0
				events := make([]*TrackingEvent, 0)
				for _, find := range finds {
					var details, place string
					var timestamp time.Time
					itemNode := htmlquery.FindOne(find, `./div[@class="item"]`)
					dateNode := htmlquery.FindOne(find, `./div[@class="date"]`)
					placeNode := htmlquery.FindOne(find, `./div[@class="name"]`)

					if itemNode != nil {
						details = TrimSpace(htmlquery.InnerText(itemNode))
					}
					if dateNode != nil {
						timestamp = adjustJpNoYear(TrimSpace(htmlquery.InnerText(dateNode)), &sSec)
					}
					if placeNode != nil {
						place = TrimSpace(htmlquery.InnerText(placeNode))
					}

					events = append(events, &TrackingEvent{
						Date:    FormatDateTime(timestamp),
						Details: details,
						Place:   place,
					})
				}
				trackingItem.Events = events
			}

			result = append(result, trackingItem)
		}
	}

	return result
}

var (
	jpNoYearDatePattern = regexp.MustCompile(`(\d{1,2})月(\d{1,2})日 (\d{1,2}):(\d{1,2})`)
)

func adjustJpNoYear(s string, sSec *int) time.Time {
	m := jpNoYearDatePattern.FindStringSubmatchIndex(s)
	if m == nil {
		return time.Time{}
	} else {
		year, cmonth, _ := time.Now().Date()
		month, _ := strconv.Atoi(s[m[2]:m[3]])
		day, _ := strconv.Atoi(s[m[4]:m[5]])
		hour, _ := strconv.Atoi(s[m[6]:m[7]])
		minute, _ := strconv.Atoi(s[m[8]:m[9]])

		if month > int(cmonth) {
			year = year - 1
		}

		sec := *sSec
		*sSec = *sSec + 1
		if *sSec > 59 {
			*sSec = 0
		}

		return time.Date(year, time.Month(month), day, hour, minute, sec, 0, time.UTC)
	}
}
