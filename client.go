package requests

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

var (
	enableLog bool = false
)

func EnableLog() {
	enableLog = true
}

type Response struct {
	headers map[string]string
	cookies map[string]string
	raw     []byte
	url     string
}

type ParseError struct {
	TargetFmt string // 期望的目标格式。
	Err       error  // 源错误。
	Raw       string // 源内容。
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("cannot parse as %s, cause=%v, raw=%v", e.TargetFmt, e.Err, e.Raw)
}

// AsJson 将结果解析为JSON对象。
// 如果解析失败则触发`panic`。
// 返回JSON对象。
func (rr *Response) AsJson() map[string]interface{} {
	result := make(map[string]interface{})
	if err := json.Unmarshal(rr.raw, &result); err != nil {
		panic(&ParseError{TargetFmt: "json", Err: err, Raw: string(rr.raw)})
	} else {
		return result
	}
}

// Scan 将结果解析为JSON对象。
// 如果解析失败则触发`panic`。
// result 待解析的结果。
func (rr *Response) Scan(result interface{}) {
	if err := json.Unmarshal(rr.raw, result); err != nil {
		panic(&ParseError{TargetFmt: "json", Err: err, Raw: string(rr.raw)})
	}
}

// AsJsonArray 将结果解析为列表形式的JSON对象。
// 如果解析失败则触发`panic`。
// 返回列表形式的对象。
func (rr *Response) AsJsonArray() []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	if err := json.Unmarshal(rr.raw, &result); err != nil {
		panic(&ParseError{TargetFmt: "json", Err: err, Raw: string(rr.raw)})
	} else {
		return result
	}
}

// AsString 将结果解析为字符串。
// 使用UTF8将返回结果解码为字符串。
// 返回解码后的结果
func (rr *Response) AsString() string {
	return string(rr.raw)
}

// AsHtml 将结果解析为HTML节点对象。
// path 表示节点对象的XPath。
// 如果结果无法被解析为HTML，那么触发`panic`。如果可以正常解析但是找不到指定的path，那么返回`nil`。
// 如果参数`path`不合法，也触发`panic`。
// 返回path指定的节点对象，如果path是空字符串，那么返回`document`节点对象。
func (rr *Response) AsHtml(path string) *html.Node {
	if doc, err := htmlquery.Parse(bytes.NewBuffer(rr.raw)); err != nil {
		panic(&ParseError{TargetFmt: "html", Err: err, Raw: string(rr.raw)})
	} else {
		if path == "" {
			return doc
		} else {
			return htmlquery.FindOne(doc, path)
		}
	}
}

// Raw 返回原始响应。
func (rr *Response) Raw() []byte {
	return rr.raw
}

// URL 返回对应的最后一次请求（多次重定向）的URL。
func (rr *Response) URL() string {
	return rr.url
}

// Header 获取逗号分隔的Header头信息。
// name HTTP Header 的名字。
// 返回头信息，如果不存在则返回空字符串，**如果存在多个则返回最后一个**。
func (rr *Response) Cookie(name string) string {
	return rr.cookies[name]
}

// Header 获取逗号分隔的Header头信息。
// name HTTP Header 的名字。
// 返回头信息，如果不存在则返回空字符串，如果存在多个则用逗号分隔。
func (rr *Response) Header(name string) string {
	return rr.headers[name]
}

var (
	proxyMap = make(map[string]*url.URL)
)

// RegisterProxy 注册代理。
// host 需要使用代理的`HOST`。
// proxy 代理对应的`URL`。
func RegisterProxy(host string, proxy *url.URL) {
	proxyMap[host] = proxy
}

var ignoreCertTransport = &http.Transport{
	Proxy: func(rr *http.Request) (*url.URL, error) {
		if pu := proxyMap[strings.ToLower(rr.Host)]; pu != nil {
			if enableLog {
				fmt.Printf("Use proxy %s for %s\n", pu, rr.URL)
			}
			return pu, nil
		} else {
			return http.ProxyFromEnvironment(rr)
		}
	},
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
}

type Request struct {
	c *http.Client

	timeout      int               // 超时时间。
	UserAgent    string            // User-Agent 头。
	accept       string            // 允许服务器返回的内容类型。
	flagCache    bool              // 是否允许服务器发送缓存
	flagReferrer bool              // 是否自动将前一个返回HTML的请求页面作为referer。
	headers      map[string]string // HTTP头部信息。
	cookies      map[string]string // Cookie信息。

	preHtmlUrl string
}

func NewRequest() *Request {
	jar, _ := cookiejar.New(nil)
	result := Request{c: &http.Client{Transport: ignoreCertTransport, Jar: jar}, flagCache: false, flagReferrer: true}
	result.reset()

	return &result
}

func (r *Request) reset() *Request {
	r.timeout = 15
	r.flagReferrer = true
	r.flagCache = false
	r.AcceptHTML()
	r.headers = nil
	r.cookies = nil

	return r
}

func (r *Request) Timeout(v int) *Request {
	r.timeout = v
	return r
}

func (r *Request) AcceptHTML() *Request {
	r.accept = "text/html,application/xhtml+xml,application/xml;q=0.9"
	return r
}

func (r *Request) AcceptJSON() *Request {
	r.accept = "application/json"
	return r
}

func (r *Request) AcceptAny() *Request {
	r.accept = "*/*"
	return r
}

func (r *Request) EnableCache() *Request {
	r.flagCache = true
	return r
}

func (r *Request) NoReferrer() *Request {
	r.flagReferrer = false
	return r
}

func (r *Request) Header(name, value string) *Request {
	name = strings.TrimSpace(name)
	if name != "" {
		if r.headers == nil {
			r.headers = make(map[string]string)
		}

		r.headers[name] = value
	}

	return r
}

func (r *Request) Cookie(name, value string) *Request {
	name = strings.TrimSpace(name)
	if name != "" {
		if r.cookies == nil {
			r.cookies = make(map[string]string)
		}

		r.cookies[name] = value
	}

	return r
}

func (r *Request) Get(url string, params map[string]string) (*Response, error) {
	return r.exec("GET", url, params, "")
}

func (r *Request) Post(url string, params map[string]string, data_ map[string]interface{}) (*Response, error) {
	r.Header("content-type", "application/x-www-form-urlencoded")
	return r.exec("POST", url, params, makeUrlEncoded(data_))
}

func (r *Request) PostJson(url string, params map[string]string, data_ interface{}) (*Response, error) {
	r.Header("content-type", "application/json")
	return r.exec("POST", url, params, makeJson(data_))
}

func (r *Request) Put(url string, params map[string]string, data_ map[string]interface{}) (*Response, error) {
	r.Header("content-type", "application/x-www-form-urlencoded")
	return r.exec("POST", url, params, makeUrlEncoded(data_))
}

func (r *Request) PutJson(url string, params map[string]string, data_ map[string]interface{}) (*Response, error) {
	return r.exec("POST", url, params, makeJson(data_))
}

func makeUrlEncoded(data_ map[string]interface{}) string {
	if len(data_) == 0 {
		return ""
	}

	rv := url.Values{}

	for k, v := range data_ {
		if lv, ok := v.([]string); ok {
			for _, lvv := range lv {
				rv.Add(k, lvv)
			}
		} else if lv, ok := v.([]int); ok {
			for _, lvv := range lv {
				rv.Add(k, strconv.Itoa(lvv))
			}
		} else {
			rv.Add(k, fmt.Sprintf("%v", v))
		}
	}

	return rv.Encode()
}

func makeJson(data_ interface{}) string {
	if rs, ok := data_.(string); ok {
		return rs
	} else if b, err := json.Marshal(data_); err != nil {
		return ""
	} else {
		return string(b)
	}
}

type HttpError struct {
	Op         string // HTTP 方法。
	URL        string // 目标URL。
	Raw        []byte // 原始响应内容。
	Err        error  // 关联的错误对象。
	StatusCode int    // 原始响应码。
}

func (e *HttpError) Error() string {
	var result string
	if e.StatusCode > 0 {
		result = fmt.Sprintf("%s %s (%d)", e.Op, e.URL, e.StatusCode)
	} else {
		result = fmt.Sprintf("%s %s (Timeout)", e.Op, e.URL)
	}
	if e.Err != nil {
		result = result + ", cause=" + e.Err.Error()
	}
	return result
}

func (e *HttpError) Timeout() bool {
	return e.StatusCode == 0
}

func (e *HttpError) CannotConn() bool {
	return e.StatusCode < 0
}

func (r *Request) exec(method, url_ string, params map[string]string, data string) (*Response, error) {
	defer r.reset()

	if rawReq, err := r.newRequest(method, url_, params, data); err != nil {
		return nil, err
	} else {
		if enableLog {
			r.logReq(rawReq, data)
		}

		t0 := time.Now()
		if resp, err := r.c.Do(rawReq); err != nil {
			ue := err.(*url.Error)
			var statusCode int
			if ue.Timeout() {
				statusCode = 0
			} else {
				statusCode = -1
			}
			return nil, &HttpError{Op: ue.Op, URL: ue.URL, Err: ue.Err, StatusCode: statusCode}
		} else {
			t1 := time.Now()
			defer resp.Body.Close()

			if enableLog {
				r.logResp(t0, t1, resp)
			}

			requestURI := resp.Request.URL.String()

			// 如果服务器响应了一个HTML页面，那么将请求地址记录到之前的HTML地址。
			if resp.StatusCode == http.StatusOK && strings.HasPrefix(resp.Header.Get("content-type"), "text/html") {
				r.preHtmlUrl = requestURI
			}

			// TODO: 此处按照HTTP头部获取charset。
			if rawData, err := ioutil.ReadAll(resp.Body); err != nil {
				return nil, &HttpError{Op: method, URL: requestURI, Err: err, StatusCode: resp.StatusCode}
			} else {
				if resp.StatusCode >= 300 {
					return nil, &HttpError{Op: method, URL: requestURI, Raw: rawData, StatusCode: resp.StatusCode}
				}

				respHeaders := make(map[string]string)
				respCookies := make(map[string]string)

				for rHeaderName, rHeaderValues := range resp.Header {
					if len(rHeaderValues) > 0 {
						respHeaders[rHeaderName] = strings.Join(rHeaderValues, ",")
					}
				}

				for _, rcookie := range resp.Cookies() {
					respCookies[rcookie.Name] = rcookie.Value
				}

				return &Response{headers: respHeaders, cookies: respCookies, raw: rawData, url: requestURI}, nil
			}
		}
	}
}

func (r *Request) newRequest(method, url_ string, params map[string]string, data string) (*http.Request, error) {
	method = strings.ToUpper(method)

	if u, err := url.Parse(url_); err != nil {
		return nil, err
	} else {
		if len(params) != 0 {
			q := u.Query()
			for k, v := range params {
				q.Add(k, v)
			}
			u.RawQuery = q.Encode()
		}

		if rawReq, err := http.NewRequest(method, u.String(), strings.NewReader(data)); err != nil {
			return nil, err
		} else {
			header := http.Header{}
			// 加入默认头部。
			if r.UserAgent == "" {
				header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/95.0.4638.54 Safari/537.36")
			} else {
				header.Set("user-agent", r.UserAgent)
			}
			if !r.flagCache {
				header.Set("pragma", "no-cache")
				header.Set("cache-control", "no-cache")
			}
			header.Set("accept", r.accept)
			if r.flagReferrer && r.preHtmlUrl != "" {
				header.Set("referer", r.preHtmlUrl)
			}
			if len(r.headers) > 0 {
				for headerName, headerValue := range r.headers {
					header.Set(headerName, headerValue)
				}
			}
			rawReq.Header = header

			// 加入Cookie。
			if len(r.cookies) > 0 {
				for cookieName, cookieValue := range r.cookies {
					rawReq.AddCookie(&http.Cookie{Name: cookieName, Value: cookieValue})
				}
			}

			r.c.Timeout = time.Duration(r.timeout) * time.Second

			return rawReq, nil
		}
	}
}

func (r *Request) logReq(req *http.Request, data string) {
	ls := make([]string, 0)

	ls = append(ls, fmt.Sprintf("%s %s HTTP1.1", req.Method, req.URL))
	for hn, hv := range req.Header {
		for _, hv0 := range hv {
			ls = append(ls, fmt.Sprintf(">> %s: %s", hn, hv0))
		}
	}
	for _, cc := range r.c.Jar.Cookies(req.URL) {
		ls = append(ls, fmt.Sprintf(">> Cookie: %s=%s", cc.Name, cc.Value))
	}
	if req.Body != nil {
		ls = append(ls, "")
		if len(data) > 512 {
			ls = append(ls, fmt.Sprintf(">> (%d bytes) %s...", len(data), data[0:512]))
		} else {
			ls = append(ls, fmt.Sprintf(">> (%d bytes) %s", len(data), data))
		}
	}

	fmt.Printf("%s\n", strings.Join(ls, "\n"))
}

func (r *Request) logResp(t0, t1 time.Time, resp *http.Response) {
	ls := make([]string, 0)

	ls = append(ls, fmt.Sprintf("%.3f seconds", t1.Sub(t0).Seconds()))
	ls = append(ls, fmt.Sprintf("<< %s", resp.Status))
	for hn, hv := range resp.Header {
		for _, hv0 := range hv {
			ls = append(ls, fmt.Sprintf("<< %s: %s", hn, hv0))
		}
	}
	if resp.Body != nil {
		ls = append(ls, "")
		ls = append(ls, "<< (data)")
	}

	fmt.Printf("%s\n", strings.Join(ls, "\n"))
}
