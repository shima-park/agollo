package agollo

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"
)

var (
	defaultClientTimeout = 90 * time.Second
)

// https://github.com/ctripcorp/apollo/wiki/%E5%85%B6%E5%AE%83%E8%AF%AD%E8%A8%80%E5%AE%A2%E6%88%B7%E7%AB%AF%E6%8E%A5%E5%85%A5%E6%8C%87%E5%8D%97
type ApolloClient interface {
	Apply(opts ...ApolloClientOption)

	Notifications(configServerURL, appID, clusterName string, notifications []Notification) (int, []Notification, error)

	// 该接口会直接从数据库中获取配置，可以配合配置推送通知实现实时更新配置。
	GetConfigsFromNonCache(configServerURL, appID, cluster, namespace string, opts ...NotificationsOption) (int, *Config, error)

	// 该接口会从缓存中获取配置，适合频率较高的配置拉取请求，如简单的每30秒轮询一次配置。
	GetConfigsFromCache(configServerURL, appID, cluster, namespace string) (Configurations, error)

	// 该接口从MetaServer获取ConfigServer列表
	GetConfigServers(metaServerURL, appID string) (int, []ConfigServer, error)
}

type Notifications []Notification

func (n Notifications) String() string {
	bytes, _ := json.Marshal(n)
	return string(bytes)
}

type Notification struct {
	NamespaceName  string `json:"namespaceName"`  // namespaceName: "application",
	NotificationID int    `json:"notificationId"` // notificationId: 107
}

type NotificationsOptions struct {
	ReleaseKey string
}

type NotificationsOption func(*NotificationsOptions)

func ReleaseKey(releaseKey string) NotificationsOption {
	return func(o *NotificationsOptions) {
		o.ReleaseKey = releaseKey
	}
}

type Config struct {
	AppID          string         `json:"appId"`          // appId: "AppTest",
	Cluster        string         `json:"cluster"`        // cluster: "default",
	NamespaceName  string         `json:"namespaceName"`  // namespaceName: "TEST.Namespace1",
	Configurations Configurations `json:"configurations"` // configurations: {Name: "Foo"},
	ReleaseKey     string         `json:"releaseKey"`     // releaseKey: "20181017110222-5ce3b2da895720e8"
}

type ConfigServer struct {
	AppName     string `json:"appName"`
	InstanceID  string `json:"instanceId"`
	HomePageURL string `json:"homepageUrl"`
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type apolloClient struct {
	Doer       Doer
	IP         string
	ConfigType string // 默认properties不需要在namespace后加后缀名，其他情况例如application.json {xml,yml,yaml,json,...}
	AccessKey  string
}

type ApolloClientOption func(*apolloClient)

func WithDoer(d Doer) ApolloClientOption {
	return func(a *apolloClient) {
		a.Doer = d
	}
}

func WithIP(ip string) ApolloClientOption {
	return func(a *apolloClient) {
		a.IP = ip
	}
}

func WithConfigType(configType string) ApolloClientOption {
	return func(a *apolloClient) {
		a.ConfigType = configType
	}
}

func WithAccessKey(accessKey string) ApolloClientOption {
	return func(a *apolloClient) {
		a.AccessKey = accessKey
	}
}

func NewApolloClient(opts ...ApolloClientOption) ApolloClient {
	c := &apolloClient{
		IP:         getLocalIP(),
		ConfigType: defaultConfigType,
		Doer: &http.Client{
			Timeout: defaultClientTimeout, // Notifications由于服务端会hold住请求60秒，所以请确保客户端访问服务端的超时时间要大于60秒。
		},
	}

	c.Apply(opts...)

	return c
}

const (
	AUTHORIZATION_FORMAT      = "Apollo %s:%s"
	DELIMITER                 = "\n"
	HTTP_HEADER_AUTHORIZATION = "Authorization"
	HTTP_HEADER_TIMESTAMP     = "Timestamp"
)

func signature(timestamp, url, accessKey string) string {

	stringToSign := timestamp + DELIMITER + url

	key := []byte(accessKey)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (c *apolloClient) httpHeader(appID, uri string) map[string]string {

	headers := map[string]string{}
	if "" == c.AccessKey {
		return headers
	}

	timestamp := fmt.Sprintf("%v", time.Now().UnixNano()/int64(time.Millisecond))
	signature := signature(timestamp, uri, c.AccessKey)

	headers[HTTP_HEADER_AUTHORIZATION] = fmt.Sprintf(AUTHORIZATION_FORMAT, appID, signature)
	headers[HTTP_HEADER_TIMESTAMP] = timestamp

	return headers
}

func (c *apolloClient) Apply(opts ...ApolloClientOption) {
	for _, opt := range opts {
		opt(c)
	}
}

func (c *apolloClient) Notifications(configServerURL, appID, cluster string, notifications []Notification) (status int, result []Notification, err error) {
	configServerURL = normalizeURL(configServerURL)
	requestURI := fmt.Sprintf("/notifications/v2?appId=%s&cluster=%s&notifications=%s",
		url.QueryEscape(appID),
		url.QueryEscape(cluster),
		url.QueryEscape(Notifications(notifications).String()),
	)
	apiURL := fmt.Sprintf("%s%s", configServerURL, requestURI)

	headers := c.httpHeader(appID, requestURI)
	status, err = c.do("GET", apiURL, headers, &result)
	return
}

func (c *apolloClient) GetConfigsFromNonCache(configServerURL, appID, cluster, namespace string, opts ...NotificationsOption) (status int, config *Config, err error) {
	var options = NotificationsOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	configServerURL = normalizeURL(configServerURL)
	requestURI := fmt.Sprintf("/configs/%s/%s/%s?releaseKey=%s&ip=%s",
		url.QueryEscape(appID),
		url.QueryEscape(cluster),
		url.QueryEscape(c.getNamespace(namespace)),
		options.ReleaseKey,
		c.IP,
	)
	apiURL := fmt.Sprintf("%s%s", configServerURL, requestURI)

	headers := c.httpHeader(appID, requestURI)
	config = new(Config)
	status, err = c.do("GET", apiURL, headers, config)
	return

}

func (c *apolloClient) GetConfigsFromCache(configServerURL, appID, cluster, namespace string) (config Configurations, err error) {
	configServerURL = normalizeURL(configServerURL)
	requestURI := fmt.Sprintf("/configfiles/json/%s/%s/%s?ip=%s",
		url.QueryEscape(appID),
		url.QueryEscape(cluster),
		url.QueryEscape(c.getNamespace(namespace)),
		c.IP,
	)
	apiURL := fmt.Sprintf("%s%s", configServerURL, requestURI)

	headers := c.httpHeader(appID, requestURI)
	config = make(Configurations)
	_, err = c.do("GET", apiURL, headers, config)
	return
}

func (c *apolloClient) GetConfigServers(metaServerURL, appID string) (int, []ConfigServer, error) {
	metaServerURL = normalizeURL(metaServerURL)
	requestURI := fmt.Sprintf("/services/config?id=%s&appId=%s", c.IP, appID)
	apiURL := fmt.Sprintf("%s%s", metaServerURL, requestURI)

	headers := c.httpHeader(appID, requestURI)
	var cfs []ConfigServer
	status, err := c.do("GET", apiURL, headers, &cfs)
	return status, cfs, err
}

func (c *apolloClient) do(method, url string, headers map[string]string, v interface{}) (status int, err error) {
	var req *http.Request
	req, err = http.NewRequest(method, url, nil)
	if err != nil {
		return
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}

	var body []byte
	status, body, err = parseResponseBody(c.Doer, req)
	if err != nil {
		return
	}

	if status == http.StatusOK {
		err = json.Unmarshal(body, v)
	}
	return
}

// 配置文件有多种格式，例如：properties、xml、yml、yaml、json等。同样Namespace也具有这些格式。在Portal UI中可以看到“application”的Namespace上有一个“properties”标签，表明“application”是properties格式的。
// 如果使用Http接口直接调用时，对应的namespace参数需要传入namespace的名字加上后缀名，如datasources.json。
func (c *apolloClient) getNamespace(namespace string) string {
	if c.ConfigType == "" || c.ConfigType == defaultConfigType {
		return namespace
	}
	return namespace + "." + c.ConfigType
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}

		}
	}
	return ""
}

func parseResponseBody(doer Doer, req *http.Request) (int, []byte, error) {
	resp, err := doer.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, body, nil
}
