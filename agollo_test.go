package agollo

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockApolloClient struct {
	notifications          func(configServerURL, appID, clusterName string, notifications []Notification) (int, []Notification, error)
	getConfigsFromNonCache func(configServerURL, appID, cluster, namespace string, opts ...NotificationsOption) (int, *Config, error)
	getConfigsFromCache    func(configServerURL, appID, cluster, namespace string) (Configurations, error)
	getConfigServers       func(metaServerURL, appID string) (int, []ConfigServer, error)
}

func (c *mockApolloClient) Apply(opts ...ApolloClientOption) {

}

func (c *mockApolloClient) Notifications(configServerURL, appID, clusterName string, notifications []Notification) (int, []Notification, error) {
	if c.notifications == nil {
		return 404, nil, nil
	}
	return c.notifications(configServerURL, appID, clusterName, notifications)
}

func (c *mockApolloClient) GetConfigsFromNonCache(configServerURL, appID, cluster, namespace string, opts ...NotificationsOption) (int, *Config, error) {
	if c.getConfigsFromNonCache == nil {
		return 404, nil, nil
	}
	return c.getConfigsFromNonCache(configServerURL, appID, cluster, namespace, opts...)
}

func (c *mockApolloClient) GetConfigsFromCache(configServerURL, appID, cluster, namespace string) (Configurations, error) {
	if c.getConfigsFromCache == nil {
		return nil, nil
	}
	return c.getConfigsFromCache(configServerURL, appID, cluster, namespace)
}

func (c *mockApolloClient) GetConfigServers(metaServerURL, appID string) (int, []ConfigServer, error) {
	if c.getConfigServers == nil {
		return 404, nil, nil
	}
	return c.getConfigServers(metaServerURL, appID)
}

func TestAgollo(t *testing.T) {
	configServerURL := "http://localhost:8080"
	appid := "test"
	cluster := "default"
	newConfigs := func() map[string]*Config {
		return map[string]*Config{
			"application": &Config{
				AppID:         appid,
				Cluster:       cluster,
				NamespaceName: "application",
				Configurations: map[string]interface{}{
					"timeout": "100",
				},
				ReleaseKey: "111",
			},
			"test.json": &Config{
				AppID:         appid,
				Cluster:       cluster,
				NamespaceName: "test.json",
				Configurations: map[string]interface{}{
					"content": `{"name":"foo","age":18}`,
				},
				ReleaseKey: "121",
			},
		}
	}

	newClient := func(configs map[string]*Config) ApolloClient {
		var lock sync.RWMutex
		var once sync.Once
		return &mockApolloClient{
			notifications: func(configServerURL, appID, clusterName string, notifications []Notification) (int, []Notification, error) {
				lock.RLock()
				rk, _ := strconv.Atoi(configs["application"].ReleaseKey)
				lock.RUnlock()

				once.Do(func() {
					rk++

					lock.Lock()
					configs["application"].ReleaseKey = fmt.Sprint(rk)
					lock.Unlock()
				})
				return 200, []Notification{
					Notification{
						NamespaceName:  "application",
						NotificationID: rk,
					},
				}, nil
			},
			getConfigsFromCache: func(configServerURL, appID, cluster, namespace string) (Configurations, error) {
				return nil, nil
			},
			getConfigsFromNonCache: func(configServerURL, appID, cluster, namespace string, opts ...NotificationsOption) (i int, config *Config, err error) {
				var options NotificationsOptions
				for _, opt := range opts {
					opt(&options)
				}

				lock.RLock()
				config, ok := configs[namespace]
				lock.RUnlock()
				if !ok {
					return 404, nil, nil
				}

				if config.ReleaseKey == options.ReleaseKey {
					return 304, nil, nil
				}

				return 200, config, nil
			},
			getConfigServers: func(metaServerURL, appID string) (i int, servers []ConfigServer, err error) {
				return 200, []ConfigServer{
					ConfigServer{HomePageURL: metaServerURL},
				}, nil
			},
		}
	}

	newBadClient := func(configs map[string]*Config) ApolloClient {
		return &mockApolloClient{
			notifications: func(configServerURL, appID, clusterName string, notifications []Notification) (int, []Notification, error) {
				return 500, nil, nil
			},
			getConfigsFromCache: func(configServerURL, appID, cluster, namespace string) (Configurations, error) {
				return nil, nil
			},
			getConfigsFromNonCache: func(configServerURL, appID, cluster, namespace string, opts ...NotificationsOption) (i int, config *Config, err error) {
				return 500, nil, nil
			},
			getConfigServers: func(metaServerURL, appID string) (i int, servers []ConfigServer, err error) {
				return 500, nil, nil
			},
		}
	}

	var tests = []struct {
		Name      string
		NewAgollo func(configs map[string]*Config) Agollo
		Test      func(a Agollo, configs map[string]*Config)
	}{
		{
			Name: "测试：预加载的namespace应该正常可获取，非预加载的namespace无法获取配置",
			NewAgollo: func(configs map[string]*Config) Agollo {
				a, err := New(configServerURL, appid, WithApolloClient(newClient(configs)), PreloadNamespaces("test.json"))
				assert.Nil(t, err)
				assert.NotNil(t, a)
				return a
			},
			Test: func(a Agollo, configs map[string]*Config) {
				for namespace, config := range configs {
					for key, expected := range config.Configurations {
						if namespace == "test.json" {
							actual := a.Get(key, WithNamespace(namespace))
							assert.Equal(t, expected, actual)
						} else {
							actual := a.Get(key, WithNamespace(namespace))
							assert.Empty(t, actual)
						}
					}
				}
			},
		},
		{
			Name: "测试：自动获取非预加载namespace时，正常读取配置配置项",
			NewAgollo: func(configs map[string]*Config) Agollo {
				a, err := New(configServerURL, appid, WithApolloClient(newClient(configs)), AutoFetchOnCacheMiss())
				assert.Nil(t, err)
				assert.NotNil(t, a)
				return a
			},
			Test: func(a Agollo, configs map[string]*Config) {
				for namespace, config := range configs {
					for key, expected := range config.Configurations {
						actual := a.Get(key, WithNamespace(namespace))
						assert.Equal(t, expected, actual)
					}
				}

				// 测试无WithNamespace配置项时读取application的配置
				key := "timeout"
				expected := configs["application"].Configurations[key]
				actual := a.Get(key)
				assert.Equal(t, expected, actual)
			},
		},
		{
			Name: "测试：初始化后 start 监听配置的情况",
			NewAgollo: func(configs map[string]*Config) Agollo {
				a, err := New(configServerURL, appid, WithApolloClient(newClient(configs)), AutoFetchOnCacheMiss())
				assert.Nil(t, err)
				assert.NotNil(t, a)
				return a
			},
			Test: func(a Agollo, configs map[string]*Config) {
				a.Start()
				defer a.Stop()

				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()

					for i := 0; i < 5; i++ {
						for namespace, config := range configs {
							for key, expected := range config.Configurations {
								actual := a.Get(key, WithNamespace(namespace))
								assert.Equal(t, expected, actual)
							}
						}
						time.Sleep(time.Second)
					}
				}()

				wg.Wait()
			},
		},
		{
			Name: "测试：容灾配置项",
			NewAgollo: func(configs map[string]*Config) Agollo {
				a, err := New(configServerURL, appid, WithApolloClient(newBadClient(configs)), AutoFetchOnCacheMiss(), FailTolerantOnBackupExists())
				assert.Nil(t, err)
				assert.NotNil(t, a)
				return a
			},
			Test: func(a Agollo, configs map[string]*Config) {
				a.Start()
				defer a.Stop()

				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()

					for i := 0; i < 5; i++ {
						for namespace, config := range configs {
							for key, expected := range config.Configurations {
								actual := a.Get(key, WithNamespace(namespace))
								assert.Equal(t, expected, actual)
							}
						}
						time.Sleep(time.Second)
					}
				}()

				wg.Wait()
			},
		},
	}

	for _, test := range tests {
		configs := newConfigs()
		test.Test(test.NewAgollo(configs), configs)
	}
}
