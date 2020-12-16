package agollo

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
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
	ConfigServerURL string // 当前访问配置使用的apollo config server的url
	AccessKey       string // 服务启动时缓存的access key
	AppID           string // appid
	RequestURI      string // 请求的uri，domain之后的路径
	Cluster         string // 请求的集群，默认情况下: default，请求GetConfigServers接口时为""
}

type SignatureFunc func(ctx *SignatureContext) Header

func DefaultSignatureFunc(ctx *SignatureContext) Header {

	headers := map[string]string{}
	if "" == ctx.AccessKey {
		return headers
	}

	timestamp := fmt.Sprintf("%v", time.Now().UnixNano()/int64(time.Millisecond))
	signature := signature(timestamp, ctx.RequestURI, ctx.AccessKey)

	headers[HTTP_HEADER_AUTHORIZATION] = fmt.Sprintf(AUTHORIZATION_FORMAT, ctx.AppID, signature)
	headers[HTTP_HEADER_TIMESTAMP] = timestamp

	return headers
}

func signature(timestamp, url, accessKey string) string {

	stringToSign := timestamp + DELIMITER + url

	key := []byte(accessKey)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
