package agollo

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

var (
	defaultConfigFilePath = "app.properties"
	defaultConfigType     = "properties"
	defaultNotificationID = -1
	defaultWatchTimeout   = 500 * time.Millisecond
	defaultAgollo         Agollo
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

type ApolloResponse struct {
	Namespace string
	OldValue  Configurations
	NewValue  Configurations
	Changes   Changes
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
	releaseKeyMap   sync.Map // key: namespace value: releaseKey
	cache           sync.Map // key: namespace value: Configurations
	initialized     sync.Map // key: namespace value: bool

	watchCh             chan *ApolloResponse // watch all namespace
	watchNamespaceChMap sync.Map             // key: namespace value: chan *ApolloResponse

	errorsCh chan *LongPollerError

	runOnce      sync.Once
	runHeartBeat sync.Once

	stop     bool
	stopCh   chan struct{}
	stopLock sync.Mutex

	backupLock sync.RWMutex
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
		AccessKey      string   `json:"accessKey,omitempty"`
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
				AccessKey(conf.AccessKey),
			},
			opts...,
		)...,
	)
}

func New(configServerURL, appID string, opts ...Option) (Agollo, error) {
	a := &agollo{
		stopCh:   make(chan struct{}),
		errorsCh: make(chan *LongPollerError),
	}

	options, err := newOptions(configServerURL, appID, opts...)
	if err != nil {
		return nil, err
	}
	a.opts = options

	return a, a.initNamespace(a.opts.PreloadNamespaces...)
}

func (a *agollo) initNamespace(namespaces ...string) error {
	var errs []error
	for _, namespace := range namespaces {
		_, found := a.initialized.LoadOrStore(namespace, true)
		if !found {
			// (1)读取配置 (2)设置初始化notificationMap
			status, _, err := a.reloadNamespace(namespace)

			// 这里没法光凭靠error==nil来判断namespace是否存在，即使http请求失败，如果开启 容错，会导致error丢失
			// 从而可能将一个不存在的namespace拿去调用getRemoteNotifications导致被hold
			a.setNotificationIDFromRemote(namespace, status == http.StatusOK)

			// 即使存在异常也需要继续初始化下去，有一些使用者会拂掠初始化时的错误
			// 期望在未来某个时间点apollo的服务器恢复过来
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

func (a *agollo) setNotificationIDFromRemote(namespace string, exists bool) {
	if !exists {
		// 不能正常获取notificationID的设置为默认notificationID
		// 为之后longPoll提供localNoticationID参数
		a.notificationMap.Store(namespace, defaultNotificationID)
		return
	}

	localNotifications := []Notification{
		{
			NotificationID: defaultNotificationID,
			NamespaceName:  namespace,
		},
	}
	// 由于apollo去getRemoteNotifications获取一个不存在的namespace的notificationID时会hold请求90秒
	// (1) 为防止意外传入一个不存在的namespace而发生上述情况，仅将成功获取配置在apollo存在的namespace,去初始化notificationID
	// (2) 此处忽略error返回，在容灾逻辑下配置能正确读取而去获取notificationid可能会返回http请求失败，防止服务不能正常容灾启动
	remoteNotifications, _ := a.getRemoteNotifications(localNotifications)
	if len(remoteNotifications) > 0 {
		for _, notification := range remoteNotifications {
			// 设置namespace初始化的notificationID
			a.notificationMap.Store(notification.NamespaceName, notification.NotificationID)
		}
	} else {
		// 不能正常获取notificationID的设置为默认notificationID
		a.notificationMap.Store(namespace, defaultNotificationID)
	}
}

func (a *agollo) reloadNamespace(namespace string) (status int, conf Configurations, err error) {
	var configServerURL string
	configServerURL, err = a.opts.Balancer.Select()
	if err != nil {
		a.log("Action", "BalancerSelect", "Error", err)
		return
	}

	var (
		config              *Config
		cachedReleaseKey, _ = a.releaseKeyMap.LoadOrStore(namespace, "")
	)
	status, config, err = a.opts.ApolloClient.GetConfigsFromNonCache(
		configServerURL,
		a.opts.AppID,
		a.opts.Cluster,
		namespace,
		ReleaseKey(cachedReleaseKey.(string)),
	)

	switch status {
	case http.StatusOK: // 正常响应
		a.cache.Store(namespace, config.Configurations)     // 覆盖旧缓存
		a.releaseKeyMap.Store(namespace, config.ReleaseKey) // 存储最新的release_key
		conf = config.Configurations

		// 备份配置
		if err = a.backup(namespace, config.Configurations); err != nil {
			a.log("BackupFile", a.opts.BackupFile, "Namespace", namespace,
				"Action", "Backup", "Error", err)
			return
		}
	case http.StatusNotModified: // 服务端未修改配置情况下返回304
		conf = a.getNamespace(namespace)
	default:
		a.log("ConfigServerUrl", configServerURL, "Namespace", namespace,
			"Action", "GetConfigsFromNonCache", "ServerResponseStatus", status,
			"Error", err)

		conf = Configurations{}

		// 异常状况下，如果开启容灾，则读取备份
		if a.opts.FailTolerantOnBackupExists {
			backupConfig, lerr := a.loadBackupByNamespace(namespace)
			if lerr != nil {
				a.log("BackupFile", a.opts.BackupFile, "Namespace", namespace,
					"Action", "loadBackupByNamespace", "Error", lerr)
				return
			}

			a.cache.Store(namespace, backupConfig)
			conf = backupConfig
			err = nil
			return
		}
	}

	return
}

func (a *agollo) Get(key string, opts ...GetOption) string {
	getOpts := a.opts.newGetOptions(opts...)

	val, found := a.GetNameSpace(getOpts.Namespace)[key]
	if !found {
		return getOpts.DefaultValue
	}

	v, _ := ToStringE(val)
	return v
}

func (a *agollo) GetNameSpace(namespace string) Configurations {
	config, found := a.cache.LoadOrStore(namespace, Configurations{})
	if !found && a.opts.AutoFetchOnCacheMiss {
		err := a.initNamespace(namespace)
		if err != nil {
			a.log("Action", "InitNamespace", "Error", err)
		}
		return a.getNamespace(namespace)
	}

	return config.(Configurations)
}

func (a *agollo) getNamespace(namespace string) Configurations {
	v, ok := a.cache.Load(namespace)
	if !ok {
		return Configurations{}
	}
	return v.(Configurations)
}

func (a *agollo) Options() Options {
	return a.opts
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

	if a.opts.EnableHeartBeat {
		a.runHeartBeat.Do(func() {
			go func() {
				timer := time.NewTimer(a.opts.HeartBeatInterval)
				defer timer.Stop()
				for !a.shouldStop() {
					select {
					case <-timer.C:
						a.heartBeat()
						timer.Reset(a.opts.HeartBeatInterval)
					case <-a.stopCh:
						return
					}
				}
			}()
		})
	}

	return a.errorsCh
}

func (a *agollo) heartBeat() {
	var configServerURL string
	configServerURL, err := a.opts.Balancer.Select()
	if err != nil {
		a.log("Action", "BalancerSelect", "Error", err)
		return
	}

	a.releaseKeyMap.Range(func(namespace, cachedReleaseKey interface{}) bool {
		var config *Config
		namespaceStr := namespace.(string)
		status, config, err := a.opts.ApolloClient.GetConfigsFromNonCache(
			configServerURL,
			a.opts.AppID,
			a.opts.Cluster,
			namespaceStr,
			ReleaseKey(cachedReleaseKey.(string)),
		)
		if err != nil {
			return true
		}
		if status == http.StatusOK {
			oldValue := a.getNamespace(namespaceStr)
			a.cache.Store(namespace, config.Configurations)
			a.releaseKeyMap.Store(namespace, config.ReleaseKey)
			if err = a.backup(namespaceStr, config.Configurations); err != nil {
				a.log("BackupFile", a.opts.BackupFile, "Namespace", namespace,
					"Action", "Backup", "Error", err)
			}
			a.sendWatchCh(namespaceStr, oldValue, config.Configurations)
			a.notificationMap.Store(namespaceStr, config.ReleaseKey)
		}
		return true
	})
}

func (a *agollo) shouldStop() bool {
	select {
	case <-a.stopCh:
		return true
	default:
		return false
	}
}

func (a *agollo) longPoll() {
	localNotifications := a.getLocalNotifications()

	// 这里有个问题是非预加载的namespace，如果在Start开启监听后才被initNamespace
	// 需要等待90秒后的下一次轮训才能收到事件通知
	notifications, err := a.getRemoteNotifications(localNotifications)
	if err != nil {
		a.sendErrorsCh("", nil, "", err)
		return
	}

	// HTTP Status: 200时，正常返回notifications数据，数组含有需要更新namespace和notificationID
	// HTTP Status: 304时，上报的namespace没有更新的修改，返回notifications为空数组，遍历空数组跳过
	for _, notification := range notifications {
		// 读取旧缓存用来给监听队列
		oldValue := a.getNamespace(notification.NamespaceName)

		// 更新namespace
		status, newValue, err := a.reloadNamespace(notification.NamespaceName)

		if err == nil {
			// Notifications 有更新，但是 GetConfigsFromNonCache 返回 304，
			// 可能是请求恰好打在尚未同步的节点上，不更新 NotificationID，等待下次再更新
			if status == http.StatusNotModified {
				continue
			}
			
			if len(oldValue.Different(newValue)) == 0 {
				// case 可能是apollo集群搭建问题
				// GetConfigsFromNonCache 返回了了一模一样的数据，但是http.status code == 200
				// 导致NotificationID更新了，但是真实的配置没有更新，而后续也不会获取到新配置，除非有新的变更触发
				continue
			}

			// 发送到监听channel
			a.sendWatchCh(notification.NamespaceName, oldValue, newValue)

			// 仅在无异常的情况下更新NotificationID，
			// 极端情况下，提前设置notificationID，reloadNamespace还未更新配置并将配置备份，
			// 访问apollo失败导致notificationid已是最新，而配置不是最新
			a.notificationMap.Store(notification.NamespaceName, notification.NotificationID)
		} else {
			a.sendErrorsCh("", notifications, notification.NamespaceName, err)
		}
	}
}

func (a *agollo) Stop() {
	a.stopLock.Lock()
	defer a.stopLock.Unlock()
	if a.stop {
		return
	}

	if a.opts.Balancer != nil {
		a.opts.Balancer.Stop()
	}

	a.stop = true
	close(a.stopCh)
}

func (a *agollo) Watch() <-chan *ApolloResponse {
	if a.watchCh == nil {
		a.watchCh = make(chan *ApolloResponse)
	}

	return a.watchCh
}

func (a *agollo) WatchNamespace(namespace string, stop chan bool) <-chan *ApolloResponse {
	watchNamespace := fixWatchNamespace(namespace)
	watchCh, exists := a.watchNamespaceChMap.LoadOrStore(watchNamespace, make(chan *ApolloResponse))
	if !exists {
		go func() {
			// 非预加载以外的namespace,初始化基础meta信息,否则没有longpoll
			err := a.initNamespace(namespace)
			if err != nil {
				watchCh.(chan *ApolloResponse) <- &ApolloResponse{
					Namespace: namespace,
					Error:     err,
				}
			}

			if stop != nil {
				select {
				case <-a.stopCh:
				case <-stop:
				}
				a.watchNamespaceChMap.Delete(watchNamespace)
			}
		}()
	}

	return watchCh.(chan *ApolloResponse)
}

func fixWatchNamespace(namespace string) string {
	// fix: 传给apollo类似test.properties这种namespace
	// 通知回来的NamespaceName却没有.properties后缀，追加.properties后缀来修正此问题
	ext := path.Ext(namespace)
	if ext == "" {
		namespace = namespace + "." + defaultConfigType
	}
	return namespace
}

func (a *agollo) sendWatchCh(namespace string, oldVal, newVal Configurations) {
	changes := oldVal.Different(newVal)
	if len(changes) == 0 {
		return
	}

	resp := &ApolloResponse{
		Namespace: namespace,
		OldValue:  oldVal,
		NewValue:  newVal,
		Changes:   changes,
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

	watchNamespace := fixWatchNamespace(namespace)
	if watchNamespaceCh, found := a.watchNamespaceChMap.Load(watchNamespace); found {
		chs = append(chs, watchNamespaceCh.(chan *ApolloResponse))
	}

	return chs
}

// sendErrorsCh 发送轮训时发生的错误信息channel，如果使用者不监听消费channel，错误会被丢弃
// 改成负载均衡机制后，不太好获取每个api使用的configServerURL有点蛋疼
func (a *agollo) sendErrorsCh(configServerURL string, notifications []Notification, namespace string, err error) {
	longPollerError := &LongPollerError{
		ConfigServerURL: configServerURL,
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
			"AppID", a.opts.AppID,
			"Cluster", a.opts.Cluster,
		},
			kvs...,
		)...,
	)
}

func (a *agollo) backup(namespace string, config Configurations) error {
	backup, err := a.loadBackup()
	if err != nil {
		backup = map[string]Configurations{}
	}

	a.backupLock.Lock()
	defer a.backupLock.Unlock()

	backup[namespace] = config
	data, err := json.Marshal(backup)
	if err != nil {
		return err
	}

	dir := filepath.Dir(a.opts.BackupFile)
	if _, err = os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
		if err != nil && !os.IsExist(err) {
			return err
		}
	}

	return ioutil.WriteFile(a.opts.BackupFile, data, 0644)
}

func (a *agollo) loadBackup() (map[string]Configurations, error) {
	a.backupLock.RLock()
	defer a.backupLock.RUnlock()

	if _, err := os.Stat(a.opts.BackupFile); err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(a.opts.BackupFile)
	if err != nil {
		return nil, err
	}

	backup := map[string]Configurations{}
	err = json.Unmarshal(data, &backup)
	if err != nil {
		return nil, err
	}
	return backup, nil
}

func (a *agollo) loadBackupByNamespace(namespace string) (Configurations, error) {
	backup, err := a.loadBackup()
	if err != nil {
		return nil, err
	}

	return backup[namespace], nil
}

// getRemoteNotifications
// 立即返回的情况：
// 1. 请求中的namespace任意一个在apollo服务器中有更新的ID会立即返回结果
// 请求被hold 90秒的情况:
// 1. 请求的notificationID和apollo服务器中的ID相等
// 2. 请求的namespace都是在apollo中不存在的
func (a *agollo) getRemoteNotifications(req []Notification) ([]Notification, error) {
	configServerURL, err := a.opts.Balancer.Select()
	if err != nil {
		a.log("ConfigServerUrl", configServerURL, "Error", err, "Action", "Balancer.Select")
		return nil, err
	}

	status, notifications, err := a.opts.ApolloClient.Notifications(
		configServerURL,
		a.opts.AppID,
		a.opts.Cluster,
		req,
	)
	if err != nil {
		a.log("ConfigServerUrl", configServerURL,
			"Notifications", req, "ServerResponseStatus", status,
			"Error", err, "Action", "LongPoll")
		return nil, err
	}

	return notifications, nil
}

func (a *agollo) getLocalNotifications() []Notification {
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
