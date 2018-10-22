package agollo

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	localIP                           = getLocalIP()
	defaultConfigFilePath             = "app.properties"
	defaultCluster                    = "default"
	defaultNamespace                  = "application"
	defaultConfigType                 = "properties"
	defaultBackupFile                 = ".agollo"
	defaultClientTimeout              = 90 * time.Second
	defaultNotificationID             = -1
	defaultLongPollInterval           = 1 * time.Second
	defaultAutoFetchOnCacheMiss       = false
	defaultFailTolerantOnBackupExists = false
	defaultWatchTimeout               = 500 * time.Millisecond

	defaultAgollo Agollo
)

type Agollo interface {
	Start() <-chan *LongPollerError
	Stop()
	Get(key string, opts ...GetOption) string
	GetNameSpace(namespace string) Configurations
	Watch() <-chan *ApolloResponse
	WatchNamespace(namespace string, stop chan bool) <-chan *ApolloResponse
	Options() Options
}

type Configurations map[string]string

type ApolloResponse struct {
	Namespace string
	OldValue  Configurations
	NewValue  Configurations
	Error     error
}

type LongPollerError struct {
	ConfigServerURL string
	AppID           string
	Cluster         string
	Notifications   []Notification
	Namespace       string // 服务响应200后去非缓存接口拉取时的namespace
	Err             error
}

type agollo struct {
	opts Options

	notificationMap sync.Map // key: namespace value: notificationId
	namespaceMap    sync.Map // key: namespace value: releaseKey
	cache           sync.Map // key: namespace value: Configurations

	watchCh             chan *ApolloResponse // watch all namespace
	watchNamespaceChMap sync.Map             // key: namespace value: chan *ApolloResponse

	errorsCh chan *LongPollerError

	runOnce  sync.Once
	stop     bool
	stopCh   chan struct{}
	stopLock sync.Mutex
}

func New(configServerURL, appID string, opts ...Option) (Agollo, error) {
	a := &agollo{
		stopCh:   make(chan struct{}),
		errorsCh: make(chan *LongPollerError),
		opts:     newOptions(opts...),
	}
	a.opts.ConfigServerURL = configServerURL
	a.opts.AppID = appID

	return a.preload()
}

func NewWithConfigFile(configFilePath string, opts ...Option) (Agollo, error) {
	f, err := os.Open(configFilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var conf struct {
		AppID          string   `json:"appId,omitempty"`
		Cluster        string   `json:"cluster,omitempty"`
		NamespaceNames []string `json:"namespaceNames,omitempty"`
		IP             string   `json:"ip,omitempty"`
	}
	if err := json.NewDecoder(f).Decode(&conf); err != nil {
		return nil, err
	}

	return New(
		conf.IP,
		conf.AppID,
		append(
			[]Option{
				Cluster(conf.Cluster),
				PreloadNamespaces(conf.NamespaceNames...),
			},
			opts...,
		)...,
	)
}

func (a *agollo) preload() (Agollo, error) {
	for _, namespace := range a.opts.PreloadNamespaces {
		_, err := a.loadConfigFromNonCache(namespace)
		if err != nil {
			if a.opts.FailTolerantOnBackupExists {
				_, err = a.loadBackup(namespace)
				if err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
	}
	return a, nil
}

func (a *agollo) Get(key string, opts ...GetOption) string {
	getOpts := newGetOptions(
		append(
			[]GetOption{
				WithNamespace(a.opts.DefaultNamespace),
			},
			opts...,
		)...,
	)

	val, found := a.GetNameSpace(getOpts.Namespace)[key]
	if !found {
		return getOpts.DefaultValue
	}

	return val
}

func (a *agollo) GetNameSpace(namespace string) Configurations {
	configs, found := a.cache.LoadOrStore(namespace, Configurations{})
	if !found {
		return a.loadNameSpace(namespace)
	}

	a.log("Namesapce", namespace, "From", "cache")

	return configs.(Configurations)
}

func (a *agollo) loadNameSpace(namespace string) Configurations {
	// 存储不存在的namespace, 之后在longPoller中拉取配置
	a.notificationMap.LoadOrStore(namespace, defaultNotificationID)

	if a.opts.AutoFetchOnCacheMiss {
		configs, err := a.loadConfigFromCache(namespace)
		if err == nil {
			a.log("Namesapce", namespace, "From", "cache-api")
			return configs
		}
	}

	if a.opts.FailTolerantOnBackupExists {
		configs, err := a.loadBackup(namespace)
		if err == nil {
			a.log("Namesapce", namespace, "From", "local")
			return configs
		}
	}
	return Configurations{}
}

func (a *agollo) Options() Options {
	return a.opts
}

func (a *agollo) Watch() <-chan *ApolloResponse {
	if a.watchCh == nil {
		a.watchCh = make(chan *ApolloResponse)
	}

	return a.watchCh
}

func (a *agollo) WatchNamespace(namespace string, stop chan bool) <-chan *ApolloResponse {
	watchCh, exists := a.watchNamespaceChMap.LoadOrStore(namespace, make(chan *ApolloResponse))
	if !exists {
		go func(stop chan bool) {
			select {
			case <-stop:
				a.watchNamespaceChMap.Delete(namespace)
			}
		}(stop)
	}

	return watchCh.(chan *ApolloResponse)
}

func (a *agollo) sendWatchCh(namespace string, oldVal, newVal Configurations) {
	resp := &ApolloResponse{
		Namespace: namespace,
		OldValue:  oldVal,
		NewValue:  newVal,
	}

	timer := time.NewTimer(defaultWatchTimeout)
	for _, watchCh := range a.getWatchChs(namespace) {
		select {
		case watchCh <- resp:
		case <-timer.C: // 防止创建全局监听或者某个namespace监听却不消费死锁问题
			timer.Reset(defaultWatchTimeout)
		}
	}
}

func (a *agollo) getWatchChs(namespace string) []chan *ApolloResponse {
	var chs []chan *ApolloResponse
	if a.watchCh != nil {
		chs = append(chs, a.watchCh)
	}

	if watchNamespaceCh, found := a.watchNamespaceChMap.Load(namespace); found {
		chs = append(chs, watchNamespaceCh.(chan *ApolloResponse))
	}

	return chs
}

func (a *agollo) sendErrorsCh(notifications []Notification, namespace string, err error) {
	longPollerError := &LongPollerError{
		ConfigServerURL: a.opts.ConfigServerURL,
		AppID:           a.opts.AppID,
		Cluster:         a.opts.Cluster,
		Notifications:   notifications,
		Namespace:       namespace,
		Err:             err,
	}
	select {
	case a.errorsCh <- longPollerError:
	default:
	}
}

func (a *agollo) log(kvs ...interface{}) {
	a.opts.Logger.Log(
		append([]interface{}{
			"[Agollo]", "",
			"ConfigServerUrl", a.opts.ConfigServerURL,
			"AppID", a.opts.AppID,
			"Cluster", a.opts.Cluster,
		},
			kvs...,
		)...,
	)
}

func (a *agollo) loadConfigFromCache(namespace string) (configurations Configurations, err error) {
	configurations, err = a.opts.ApolloClient.GetConfigsFromCache(
		a.opts.ConfigServerURL,
		a.opts.AppID,
		a.opts.Cluster,
		namespace)
	if err != nil {
		a.log("Namespace", namespace, "Action", "LoadConfigFromCache", "Error", err.Error())
		return
	}

	err = a.handleConfig(namespace, configurations)

	return
}

func (a *agollo) loadConfigFromNonCache(namespace string) (configurations Configurations, err error) {

	var (
		status              int
		config              *Config
		cachedReleaseKey, _ = a.namespaceMap.LoadOrStore(namespace, "")
	)
	status, config, err = a.opts.ApolloClient.GetConfigsFromNonCache(
		a.opts.ConfigServerURL,
		a.opts.AppID,
		a.opts.Cluster,
		namespace,
		ReleaseKey(cachedReleaseKey.(string)),
	)
	if err != nil {
		a.log("Namespace", namespace, "Action", "LoadConfigFromNonCache", "Error", err.Error())
		return
	}

	if status == http.StatusOK {
		configurations = config.Configurations
		a.namespaceMap.Store(namespace, config.ReleaseKey)
		err = a.handleConfig(namespace, config.Configurations)
		return
	}

	return
}

func (a *agollo) handleConfig(namespace string, configurations Configurations) error {
	// 读取旧缓存用来给监听队列
	oldValue := a.GetNameSpace(namespace)
	// 覆盖旧缓存
	a.cache.Store(namespace, configurations)

	// 发送到监听channel
	a.sendWatchCh(namespace, oldValue, configurations)
	// 备份配置
	return a.backup()
}

func (a *agollo) backup() error {
	backup := map[string]Configurations{}
	a.cache.Range(func(key, val interface{}) bool {
		k, _ := key.(string)
		conf, _ := val.(Configurations)
		backup[k] = conf
		return true
	})

	data, err := json.Marshal(backup)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(a.opts.BackupFile, data, 0644)
}

func (a *agollo) loadBackup(specifyNamespace string) (Configurations, error) {
	data, err := ioutil.ReadFile(a.opts.BackupFile)
	if err != nil {
		return nil, err
	}

	backup := map[string]Configurations{}
	err = json.Unmarshal(data, &backup)
	if err != nil {
		return nil, err
	}

	for namespace, configs := range backup {
		if namespace == specifyNamespace {
			a.cache.Store(namespace, configs)
			return configs, nil
		}
	}

	return nil, nil
}

func (a *agollo) longPoll() {
	notifications := a.notifications()
	status, notifications, err := a.opts.ApolloClient.Notifications(
		a.opts.ConfigServerURL,
		a.opts.AppID,
		a.opts.Cluster,
		notifications,
	)
	if err != nil {
		a.log("Notifications", Notifications(a.notifications()).String(),
			"Error", err.Error(), "Action", "LongPoll")
		a.sendErrorsCh(notifications, "", err)
	}

	if status == http.StatusOK {
		// 服务端判断没有改变，不会返回结果,这个时候不需要修改，遍历空数组跳过
		for _, notification := range notifications {
			_, err = a.loadConfigFromNonCache(notification.NamespaceName)
			if err == nil {
				a.notificationMap.Store(notification.NamespaceName, notification.NotificationID)
				continue
			} else {
				a.sendErrorsCh(notifications, notification.NamespaceName, err)
			}
		}
	}
}

func (a *agollo) notifications() []Notification {
	var notifications []Notification

	a.notificationMap.Range(func(key, val interface{}) bool {
		k, _ := key.(string)
		v, _ := val.(int)
		notifications = append(notifications, Notification{
			NamespaceName:  k,
			NotificationID: v,
		})

		return true
	})
	return notifications
}

// 启动goroutine去轮训apollo通知接口
func (a *agollo) Start() <-chan *LongPollerError {
	a.runOnce.Do(func() {
		go func() {
			timer := time.NewTimer(a.opts.LongPollerInterval)
			defer timer.Stop()

			for !a.shouldStop() {
				select {
				case <-timer.C:
					a.longPoll()
					timer.Reset(a.opts.LongPollerInterval)
				case <-a.stopCh:
					return
				}
			}
		}()
	})

	return a.errorsCh
}

func (a *agollo) Stop() {
	a.stopLock.Lock()
	defer a.stopLock.Unlock()
	if a.stop {
		return
	}
	a.stop = true
	close(a.stopCh)
}

func (a *agollo) shouldStop() bool {
	select {
	case <-a.stopCh:
		return true
	default:
		return false
	}
}

func Init(configServerURL, appID string, opts ...Option) (err error) {
	defaultAgollo, err = New(configServerURL, appID, opts...)
	return
}

func InitWithConfigFile(configFilePath string, opts ...Option) (err error) {
	defaultAgollo, err = NewWithConfigFile(configFilePath, opts...)
	return
}

func InitWithDefaultConfigFile(opts ...Option) error {
	return InitWithConfigFile(defaultConfigFilePath, opts...)
}

func Start() <-chan *LongPollerError {
	return defaultAgollo.Start()
}

func Stop() {
	defaultAgollo.Stop()
}

func Get(key string, opts ...GetOption) string {
	return defaultAgollo.Get(key, opts...)
}

func GetNameSpace(namespace string) Configurations {
	return defaultAgollo.GetNameSpace(namespace)
}

func Watch() <-chan *ApolloResponse {
	return defaultAgollo.Watch()
}

func WatchNamespace(namespace string, stop chan bool) <-chan *ApolloResponse {
	return defaultAgollo.WatchNamespace(namespace, stop)
}

func GetAgollo() Agollo {
	return defaultAgollo
}
