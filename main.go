package requests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	securityText = "1qazty@"
)

type Agent struct {
	MaxOrdersPerPage int // 每个查询页面可以查询的单号。

	Execute func(r *Request, trackingNoList []string, lan, postcode, dest, date string) []*TrackingItem // 执行查询的入口。
}

type response struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Items   []*TrackingItem `json:"items"`
}

type TrackingItem struct {
	CMess      string           `json:"cMess"`
	TrackingNo string           `json:"trackingNo"`
	Code       int              `json:"code"`
	CodeMg     string           `json:"codeMg"`
	Events     []*TrackingEvent `json:"trackingEventList"`
}

func (t *TrackingItem) String() string {
	return fmt.Sprintf("tracking-item{code: %d, code-mg: %#v, c-mess: %#v, tracking-no: %#v, events: %v}", t.Code, t.CodeMg, t.CMess, t.TrackingNo, t.Events)
}

type TrackingEvent struct {
	Date    string `json:"date"`
	Place   string `json:"place"`
	Details string `json:"details"`
}

func (t *TrackingEvent) String() string {
	return fmt.Sprintf("event{date: %s, place: %#v, details: %#v}", t.Date, t.Place, t.Details)
}

var (
	agents = make(map[string]*Agent)
)

func NewTrackingItem(trackingNo string) *TrackingItem {
	result := TrackingItem{
		CMess:      "success",
		TrackingNo: trackingNo,
		Code:       1,
		CodeMg:     "",
	}

	return &result
}

// RegisterAgent 注册查询代理。
// carrierCode 运输商编号, 如果是空则表示默认代理。
// agent 代理。
func RegisterAgent(carrierCode string, agent *Agent) {
	if agent == nil {
		panic(fmt.Errorf("illegal nil agent for %s", carrierCode))
	}
	carrierCode = strings.TrimSpace(strings.ToLower(carrierCode))
	agents[carrierCode] = agent
}

// 有返回，数据空
// {"code": 205, "codeMg": "$数据为空$(fastway: json.error  ctt  td.innerText)", "trackingEventList": []}

// 无返回，其它错误
// {"code": 407, "codeMg": "$错误内容$", "trackingEventList": []}

// 无返回，网站无法访问
// {"code": 408, "codeMg": "$超时$", "trackingEventList": []}

// 无返回，读取响应超时
// {"code": 409, "codeMg": "$超时$", "trackingEventList": []}

// 有返回，无法读取内容，可能服务器错误或者编码格式错误。
// {"code": 410, "codeMg": "$超时$", "trackingEventList": []}

// 有返回，可以读取内容，但是不是期望的格式。
// {"code": 206, "codeMg": "$解析失败$", "trackingEventList": []}

// 有返回，正常数据
// {"code": 1,  "trackingEventList": []}

// 启动查询。
func Run(w http.ResponseWriter, r *http.Request) {
	defer recover500(w)

	if enableLog {
		fmt.Printf("Entry: %s %s\n", r.Method, r.URL)
	}

	carrierCode := strings.TrimSpace(r.FormValue("carriercode"))
	nums := strings.TrimSpace(r.FormValue("nums"))
	lan := strings.TrimSpace(r.FormValue("lan"))
	postcode := strings.TrimSpace(r.FormValue("postcode"))
	dest := strings.TrimSpace(r.FormValue("dest"))
	date := strings.TrimSpace(r.FormValue("date"))
	timestamp := strings.TrimSpace(r.FormValue("timestamp"))
	token := strings.TrimSpace(r.FormValue("token"))

	if timestamp != "" {
		timestamp_, _ := strconv.ParseInt(timestamp, 10, 64)
		stt := time.Unix(timestamp_, 0)
		if int64(time.Since(stt).Minutes()) > 5 {
			// 时间戳已过期，可能是伪造的。
			panic(fmt.Errorf("illegal timestamp"))
		}
		if !VerifyWithMd5(token, carrierCode, nums, lan, postcode, dest, date, timestamp, securityText) {
			panic(fmt.Errorf("illegal token"))
		}
	}

	trackingNoList := strings.Split(nums, ",")
	for i := range trackingNoList {
		trackingNoList[i] = strings.TrimSpace(trackingNoList[i])
	}
	if len(trackingNoList) == 0 {
		panic(fmt.Errorf("no nums"))
	}

	carrierCode = strings.ToLower(carrierCode)
	agent := agents[carrierCode]
	if agent == nil {
		agent = agents[""]
		if agent == nil {
			panic(fmt.Errorf("unknown carriercode: %s", carrierCode))
		}
	}

	if agent.MaxOrdersPerPage == 0 {
		agent.MaxOrdersPerPage = 1
	}

	if agent.Execute == nil {
		panic("func Execute is nil")
	}

	// 自动开启协程查询。

	parallel := len(trackingNoList) / agent.MaxOrdersPerPage
	if len(trackingNoList)%agent.MaxOrdersPerPage != 0 {
		parallel++
	}

	result := make([]*TrackingItem, 0)
	resultLocker := sync.Mutex{}

	wg := sync.WaitGroup{}
	wg.Add(parallel)

	executeAgent := func(trackigNoList []string, trackingItemList *[]*TrackingItem) {
		defer func() {
			err := recover()
			if err != nil {
				var errCode int
				var errMessage string

				if re, ok := err.(*requestError); ok {
					if re.Timeout() {
						// 超时错误。
						errCode = 409
						errMessage = fmt.Sprintf("$超时: %s$", re)
					} else if re.CannotConn() {
						// 无法连接。
						errCode = 408
						errMessage = fmt.Sprintf("$网站无法访问: %s$", re)
					} else {
						// 其它HTTP错误。
						errCode = 410
						errMessage = fmt.Sprintf("$其它: %s$", re)
					}
				} else if pe, ok := err.(*parseError); ok {
					// 可以获取结果，但是无法解析到目标格式。
					errCode = 206
					errMessage = fmt.Sprintf("$解析错误: %s$", pe)
				} else {
					// 不是HTTP错误或者解析错误，可能是其它原因。
					errCode = 407
					errMessage = fmt.Sprintf("$未知: %v\n%s$", err, string(debug.Stack()))
				}

				for _, trackingNo := range trackigNoList {
					*trackingItemList = append(*trackingItemList, &TrackingItem{
						Code:       errCode,
						CodeMg:     errMessage,
						TrackingNo: trackingNo,
						Events:     []*TrackingEvent{},
					})
				}
			}
		}()

		req := NewRequest()

		result := agent.Execute(req, trackingNoList, lan, postcode, dest, date)
		*trackingItemList = append(*trackingItemList, result...)
	}

	for i := 0; i < parallel; i++ {
		go func() {
			// 默认客户端，要求服务器不缓存内容，自动携带上一个页面作为Referer，请求数据的超时时间15秒，要求服务器返回HTML页面。
			req := &Request{flagCache: false, flagReferrer: true}
			req.reset()

			p0 := i * parallel
			p1 := p0 + agent.MaxOrdersPerPage
			if p1 > len(trackingNoList) {
				p1 = len(trackingNoList)
			}

			br := make([]*TrackingItem, 0)
			executeAgent(trackingNoList[p0:p1], &br)

			resultLocker.Lock()
			defer resultLocker.Unlock()
			result = append(result, br...)

			wg.Done()
		}()
		time.Sleep(100 * time.Millisecond)
	}

	wg.Wait()

	if content, err := json.Marshal(response{
		Code:    200,
		Message: "",
		Items:   result,
	}); err != nil {
		panic(err)
	} else {
		w.Header().Set("content-type", "application/json")
		w.Header().Set("content-length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(200)
		w.Write(content)
	}
}

func recover500(w http.ResponseWriter) {
	if err := recover(); err != nil {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(fmt.Sprintf("%v\n%s", err, debug.Stack())))
	}
}
