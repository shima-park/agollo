package agollo

import (
	"encoding/json"
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
