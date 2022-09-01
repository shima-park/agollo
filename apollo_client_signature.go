package agollo

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const (
	AUTHORIZATION_FORMAT      = "Apollo %s:%s"
	DELIMITER                 = "\n"
	HTTP_HEADER_AUTHORIZATION = "Authorization"
	HTTP_HEADER_TIMESTAMP     = "Timestamp"
)

type Header map[string]string

type SignatureContext struct {
	AppID           string // appid
	AccessKey       string // 服务启动时缓存的access key
	ConfigServerURL string // 当前访问配置使用的apollo config server的url
	RequestURI      string // 请求的uri，domain之后的路径
	Cluster         string // 请求的集群，默认情况下: default，请求GetConfigServers接口时为""
}

type SignatureFunc func(ctx *SignatureContext) Header

func DefaultSignatureFunc(ctx *SignatureContext) Header {
	if ctx.AppID == "" || ctx.AccessKey == "" {
		return nil
	}

	apiURL := fmt.Sprintf("%s%s", ctx.ConfigServerURL, ctx.RequestURI)
	timestamp := strconv.Itoa(int(time.Now().UnixMilli()))
	signature := signature(timestamp, getPathWithQuery(apiURL), ctx.AccessKey)

	return map[string]string{
		HTTP_HEADER_AUTHORIZATION: fmt.Sprintf(AUTHORIZATION_FORMAT, ctx.AppID, signature),
		HTTP_HEADER_TIMESTAMP:     timestamp,
	}
}

func getPathWithQuery(uri string) string {
	r, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	r.Scheme = ""
	r.Host = ""
	return r.String()
}

func signature(timestamp, url, accessKey string) string {
	mac := hmac.New(sha1.New, []byte(accessKey))
	_, _ = mac.Write([]byte(timestamp + DELIMITER + url))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
