package agollo

import (
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

func NewApolloClient(opts ...ApolloClientOption) ApolloClient {
	c := &apolloClient{}
	for _, opt := range opts {
		opt(c)
	}

	if c.Doer == nil {
		c.Doer = &http.Client{
			Timeout: defaultClientTimeout, // Notifications由于服务端会hold住请求60秒，所以请确保客户端访问服务端的超时时间要大于60秒。
		}
	}

	if c.IP == "" {
		c.IP = getLocalIP()
	}

	if c.ConfigType == "" {
		c.ConfigType = defaultConfigType
	}

	return c
}

func (c *apolloClient) Notifications(configServerURL, appID, cluster string, notifications []Notification) (status int, result []Notification, err error) {
	configServerURL = normalizeURL(configServerURL)
	url := fmt.Sprintf("%s/notifications/v2?appId=%s&cluster=%s&notifications=%s",
		configServerURL,
		url.QueryEscape(appID),
		url.QueryEscape(cluster),
		url.QueryEscape(Notifications(notifications).String()),
	)

	status, err = c.do("GET", url, &result)
	return
}

func (c *apolloClient) GetConfigsFromNonCache(configServerURL, appID, cluster, namespace string, opts ...NotificationsOption) (status int, config *Config, err error) {
	var options = NotificationsOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	configServerURL = normalizeURL(configServerURL)
	url := fmt.Sprintf("%s/configs/%s/%s/%s?releaseKey=%s&ip=%s",
		configServerURL,
		url.QueryEscape(appID),
		url.QueryEscape(cluster),
		url.QueryEscape(c.getNamespace(namespace)),
		options.ReleaseKey,
		c.IP,
	)

	config = new(Config)
	status, err = c.do("GET", url, config)
	return

}

func (c *apolloClient) GetConfigsFromCache(configServerURL, appID, cluster, namespace string) (config Configurations, err error) {
	configServerURL = normalizeURL(configServerURL)
	url := fmt.Sprintf("%s/configfiles/json/%s/%s/%s?ip=%s",
		configServerURL,
		url.QueryEscape(appID),
		url.QueryEscape(cluster),
		url.QueryEscape(c.getNamespace(namespace)),
		c.IP,
	)

	config = make(Configurations)
	_, err = c.do("GET", url, config)
	return
}

func (c *apolloClient) GetConfigServers(metaServerURL, appID string) (int, []ConfigServer, error) {
	metaServerURL = normalizeURL(metaServerURL)
	url := fmt.Sprintf("%s/services/config?id=%s&appId=%s", metaServerURL, c.IP, appID)
	var cfs []ConfigServer
	status, err := c.do("GET", url, &cfs)
	return status, cfs, err
}

func (c *apolloClient) do(method, url string, v interface{}) (status int, err error) {
	var req *http.Request
	req, err = http.NewRequest(method, url, nil)
	if err != nil {
		return
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
