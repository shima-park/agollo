package agollo

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
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

type testCase struct {
	Name string
	Test func(configs map[string]*Config)
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

	rand.Seed(time.Now().Unix())

	newClient := func(configs map[string]*Config) ApolloClient {
		return &mockApolloClient{
			notifications: func(configServerURL, appID, clusterName string, notifications []Notification) (int, []Notification, error) {

				rk, _ := strconv.Atoi(configs["application"].ReleaseKey)
				n := rand.Intn(2)
				if n%2 == 0 {
					rk++
					configs["application"].ReleaseKey = fmt.Sprint(rk)
				}
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

				config, ok := configs[namespace]

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
	_ = newBadClient
	var tests = []testCase{
		{
			Name: "测试：预加载的namespace应该正常可获取，非预加载的namespace无法获取配置",
			Test: func(configs map[string]*Config) {
				backupfile, err := ioutil.TempFile("", "backup")
				if err != nil {
					log.Fatal(err)
				}
				defer os.Remove(backupfile.Name())

				a, err := New(configServerURL, appid,
					WithApolloClient(newClient(configs)),
					PreloadNamespaces("test.json"),
					BackupFile(backupfile.Name()),
				)
				assert.Nil(t, err)
				assert.NotNil(t, a)

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
			Test: func(configs map[string]*Config) {
				backupfile, err := ioutil.TempFile("", "backup")
				if err != nil {
					log.Fatal(err)
				}
				defer os.Remove(backupfile.Name())

				a, err := New(configServerURL, appid,
					WithApolloClient(newClient(configs)),
					AutoFetchOnCacheMiss(),
					WithLogger(NewLogger(LoggerWriter(os.Stdout))),
					BackupFile(backupfile.Name()),
				)
				assert.Nil(t, err)
				assert.NotNil(t, a)

				for namespace, config := range configs {
					for key, expected := range config.Configurations {
						actual := a.Get(key, WithNamespace(namespace))
						assert.Equal(t, expected, actual,
							"configs: %v, agollo: %v, Namespace: %s, Key: %s",
							configs, a.GetNameSpace(namespace), namespace, key)
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
			Test: func(configs map[string]*Config) {
				backupfile, err := ioutil.TempFile("", "backup")
				if err != nil {
					log.Fatal(err)
				}
				defer os.Remove(backupfile.Name())

				a, err := New(configServerURL, appid,
					WithApolloClient(newClient(configs)),
					AutoFetchOnCacheMiss(),
					BackupFile(backupfile.Name()),
				)
				assert.Nil(t, err)
				assert.NotNil(t, a)

				a.Start()
				defer a.Stop()

				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()

					for i := 0; i < 3; i++ {
						for namespace, config := range configs {
							for key, expected := range config.Configurations {
								actual := a.Get(key, WithNamespace(namespace))
								assert.Equal(t, expected, actual)
							}
						}
						time.Sleep(time.Second / 2)
					}
				}()

				wg.Wait()
			},
		},
		{
			Name: "测试：容灾配置项",
			Test: func(configs map[string]*Config) {
				backupfile, err := ioutil.TempFile("", "backup")
				if err != nil {
					log.Fatal(err)
				}
				defer os.Remove(backupfile.Name())

				enc := json.NewEncoder(backupfile)

				backup := map[string]Configurations{}
				for _, config := range configs {
					backup[config.NamespaceName] = config.Configurations
				}

				err = enc.Encode(backup)
				if err != nil {
					log.Fatal(err)
				}

				a, err := New(configServerURL, appid,
					WithApolloClient(newBadClient(configs)),
					AutoFetchOnCacheMiss(),
					FailTolerantOnBackupExists(),
					BackupFile(backupfile.Name()),
				)
				assert.Nil(t, err)
				assert.NotNil(t, a)

				a.Start()
				defer a.Stop()

				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()

					for i := 0; i < 3; i++ {
						for namespace, config := range configs {
							for key, expected := range config.Configurations {
								actual := a.Get(key, WithNamespace(namespace))
								assert.Equal(t, expected, actual, "%v %s", a.GetNameSpace(namespace), namespace)
							}
						}
						time.Sleep(time.Second / 2)
					}
				}()

				wg.Wait()
			},
		},
	}

	var wg sync.WaitGroup
	wg.Add(len(tests))
	for _, test := range tests {
		go func(test testCase) {
			defer wg.Done()
			t.Log("Test case:", test.Name)
			configs := newConfigs()
			test.Test(configs)
		}(test)
	}

	wg.Wait()
}
