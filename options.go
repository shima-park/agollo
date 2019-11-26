package agollo

import (
	"time"
)

var (
	defaultCluster                    = "default"
	defaultNamespace                  = "application"
	defaultBackupFile                 = ".agollo"
	defaultAutoFetchOnCacheMiss       = false
	defaultFailTolerantOnBackupExists = false
	defaultLongPollInterval           = 1 * time.Second
)

type Options struct {
	ConfigServerURL            string        // apollo 服务地址
	AppID                      string        // appid
	Cluster                    string        // 默认的集群名称，默认：default
	DefaultNamespace           string        // 默认的命名空间，默认：application
	PreloadNamespaces          []string      // 预加载命名空间，默认：application
	ApolloClient               ApolloClient  // apollo HTTP api实现
	Logger                     Logger        // 需要日志需要设置实现，或者注入有效的io.Writer，默认: ioutil.Discard
	AutoFetchOnCacheMiss       bool          // 自动获取非预设以外的namespace的配置，默认：false
	LongPollerInterval         time.Duration // 轮训间隔时间，默认：1s
	BackupFile                 string        // 备份文件存放地址，默认：.agollo
	FailTolerantOnBackupExists bool          // 服务器连接失败时允许读取备份，默认：false
}

func newOptions(opts ...Option) Options {
	var options = Options{
		AutoFetchOnCacheMiss:       defaultAutoFetchOnCacheMiss,
		FailTolerantOnBackupExists: defaultFailTolerantOnBackupExists,
	}
	for _, opt := range opts {
		opt(&options)
	}

	if options.Cluster == "" {
		options.Cluster = defaultCluster
	}

	if options.DefaultNamespace == "" {
		options.DefaultNamespace = defaultNamespace
	}

	if len(options.PreloadNamespaces) == 0 {
		options.PreloadNamespaces = []string{defaultNamespace}
	} else {
		if !stringInSlice(defaultNamespace, options.PreloadNamespaces) {
			PreloadNamespaces(defaultNamespace)(&options)
		}
	}

	if options.ApolloClient == nil {
		options.ApolloClient = NewApolloClient()
	}

	if options.Logger == nil {
		options.Logger = NewLogger()
	}

	if options.LongPollerInterval <= time.Duration(0) {
		options.LongPollerInterval = defaultLongPollInterval
	}

	if options.BackupFile == "" {
		options.BackupFile = defaultBackupFile
	}

	return options
}

type Option func(*Options)

func Cluster(cluster string) Option {
	return func(o *Options) {
		o.Cluster = cluster
	}
}

func DefaultNamespace(defaultNamespace string) Option {
	return func(o *Options) {
		o.DefaultNamespace = defaultNamespace
	}
}

func PreloadNamespaces(namespaces ...string) Option {
	return func(o *Options) {
		o.PreloadNamespaces = append(o.PreloadNamespaces, namespaces...)
	}
}

func WithApolloClient(c ApolloClient) Option {
	return func(o *Options) {
		o.ApolloClient = c
	}
}

func WithLogger(l Logger) Option {
	return func(o *Options) {
		o.Logger = l
	}
}

func AutoFetchOnCacheMiss() Option {
	return func(o *Options) {
		o.AutoFetchOnCacheMiss = true
	}
}

func LongPollerInterval(i time.Duration) Option {
	return func(o *Options) {
		o.LongPollerInterval = i
	}
}

func BackupFile(backupFile string) Option {
	return func(o *Options) {
		o.BackupFile = backupFile
	}
}

func FailTolerantOnBackupExists() Option {
	return func(o *Options) {
		o.FailTolerantOnBackupExists = true
	}
}

type GetOptions struct {
	DefaultValue string
	Namespace    string
}

func newGetOptions(opts ...GetOption) GetOptions {
	var getOpts GetOptions
	for _, opt := range opts {
		opt(&getOpts)
	}
	return getOpts
}

type GetOption func(*GetOptions)

func WithDefault(defVal string) GetOption {
	return func(o *GetOptions) {
		o.DefaultValue = defVal
	}
}

func WithNamespace(namespace string) GetOption {
	return func(o *GetOptions) {
		o.Namespace = namespace
	}
}
