# Agollo - Go Client for Apollo

携程Apollo Golang版客户端

## 快速开始
### 获取安装
```
go get -u github.com/shima-park/agollo
```

## Features
* 实时同步配置
* 配置文件容灾
* 零依赖
* 支持多namespace
* 提供Viper配置库的apollo插件

## 示例

### 读取配置
此示例场景适用于程序启动时读取一次。不会额外启动goroutine同步配置
```
func main(){
    a, err := agollo.New("localhost:8080", "your_appid", agollo.AutoFetchOnCacheMiss())
	if err != nil {
		panic(err)
	}

        // 获取your_appid下
	fmt.Println(
		a.Get("foo"),                // namespace为application中配置项foo的value
		a.GetNameSpace("test.json"), // namespace为test.json的所有配置项
		a.Get("foo", agollo.WithDefault("bar")), // foo这个key不存在时返回bar
		a.Get("foo", agollo.WithNamespace("other_namespace")), // namespace为other_namespace, key为foo的value
	)
}
```

### 实时同步配置
启动一个goroutine实时同步配置, errorCh返回notifications/v2非httpcode(200)的错误信息
```
a, err := agollo.New("localhost:8080", "your_appid", agollo.AutoFetchOnCacheMiss())
// error handle...

errorCh := a.Start()
// 或者忽略错误处理直接 a.Start()
```

### 配置监听
监听所有namespace配置变更事件
```
a, err := agollo.New("localhost:8080", "your_appid", agollo.AutoFetchOnCacheMiss())
// error handle...

watchCh := a.Watch()

...
case resp := <-watchCh:
		fmt.Println(
		    "Namesapce:", resp.Namesapce,
		    "OldValue:", resp.OldValue,
		    "NewValue:", resp.NewValue,
		    "Error:", resp.Error,
		)
...
```
### 配置文件容灾
初始化时增加agollo.FailTolerantOnBackupExists()即可，
在连接apollo失败时，如果在配置的目录下存在.agollo备份配置，会读取备份在服务器无法连接的情况下
```
a, err := agollo.New("localhost:8080", "your_appid", 
		agollo.FailTolerantOnBackupExists(),
		// other options...
	)
// error handle...
```

### 支持多namespace
初始化时增加agollo.AutoFetchOnCacheMiss() 当本地缓存中namespace不存在时，尝试去apollo缓存接口去获取
```
a, err := agollo.New("localhost:8080", "your_appid", 
		agollo.AutoFetchOnCacheMiss(),
		// other options...
	)
// error handle...

appNS, aNS, bNS := a.GetNameSpace("application"), a.GetNameSpace("Namespace_A"), a.GetNameSpace("Namespace_B")

a.Get("foo") // 默认从application这个namespace中查找配置项
a.Get("foo", agollo.WithNamespace("Namespace_A"))
a.Get("foo", agollo.WithNamespace("Namespace_B"))
// ...
```

或者初始化时增加agollo.PreloadNamespaces("Namespace_A", "Namespace_B", ...)预加载这几个namesapce的配置  
```
a, err := agollo.New("localhost:8080", "your_appid", 
		agollo.PreloadNamespaces("Namespace_A", "Namespace_B", ...),
		// other options...
	)
// error handle...
```

当然两者结合使用也是可以的。
```
a, err := agollo.New("localhost:8080", "your_appid", 
		agollo.PreloadNamespaces("Namespace_A", "Namespace_B", ...),
		agollo.AutoFetchOnCacheMiss(),
		// other options...
	)
// error handle...
```

### 如何支持多cluster
初始化时增加agollo.Cluster("your_cluster")，并创建多个Agollo接口实例[issue](https://github.com/shima-park/agollo/issues/1)
```
cluster_a, err := agollo.New("localhost:8080", "your_appid", 
		agollo.Cluster("cluster_a"),
		agollo.AutoFetchOnCacheMiss(),
		// other options...
	)

cluster_b, err := agollo.New("localhost:8080", "your_appid", 
		agollo.Cluster("cluster_b"),
		agollo.AutoFetchOnCacheMiss(),
		// other options...
	)
	
cluster_a.Get("foo")
cluster_b.Get("foo")
// ...
```

### 初始化方式

package级别初始化，影响默认对象和package提供的静态方法。适用于不做对象传递，单一AppID的场景
```
agollo.Init(configServerURL, appID string, opts ...Option) (err error)
agollo.InitWithConfigFile(configFilePath string, opts ...Option) (err error)
// 读取当前目录下app.properties，适用于原始apollo定义的读取固定配置文件同学
agollo.InitWithDefaultConfigFile(opts ...Option) error  
```

新建对象初始化，返回独立的Agollo接口对象。互相之间不会影响，适用于多AppID，Cluser, ConfigServer配置读取
[issue](https://github.com/shima-park/agollo/issues/1)
```
agollo.New(configServerURL, appID string, opts ...Option) (Agollo, error)
agollo.NewWithConfigFile(configFilePath string, opts ...Option) (Agollo, error)
```

### 初始化时可选配置项
更多配置请见[options.go](https://github.com/shima-park/agollo/blob/master/options.go)
```
        // 打印日志，打印日志注入有效的io.Writer，默认: ioutil.Discard
	agollo.WithLogger(agollo.NewLogger(agollo.LoggerWriter(os.Stdout))), 
	
	// 默认的集群名称，默认：default
	agollo.Cluster(cluster),
	
	// 预先加载的namespace列表，如果是通过配置启动，会在app.properties配置的基础上追加
	agollo.PreloadNamespaces("Namespace_A", "Namespace_B", ...),                          
	
	// 在配置未找到时，去apollo的带缓存的获取配置接口，获取配置
	agollo.AutoFetchOnCacheMiss(),                                
	
	// 备份文件存放地址，默认：当前目录下/.agollo，一般结合FailTolerantOnBackupExists使用
	agollo.BackupFile("/tmp/xxx/.agollo")                            
	// 在连接apollo失败时，如果在配置的目录下存在.agollo备份配置，会读取备份在服务器无法连接的情况下
	agollo.FailTolerantOnBackupExists(),            
)
```

### 详细特性展示
请将example/sample下app.properties修改为你本地或者测试的apollo配置。  
[示例代码](https://github.com/shima-park/agollo/blob/master/examples/sample/main.go)

## 结合viper使用，提高配置读取舒适度
例如apollo中有以下配置:
```
appsalt = xxx
database.driver = mysql
database.host = localhost
database.port = 3306
database.timeout = 5s
// ...
```

示例代码:  
```
import (
    "fmt"
	"github.com/shima-park/agollo/viper-remote"
	"github.com/spf13/viper"
)

type Config struct {
	AppSalt string         `mapstructure:"appsalt"`
	DB      DatabaseConfig `mapstructure:"database"`
}

type DatabaseConfig struct {
	Driver   string        `mapstructure:"driver"`
	Host     string        `mapstructure:"host"`
	Port     int           `mapstructure:"port"`
	Timeout time.Duration  `mapstructure:"timeout"`
	// ...
}

func main(){
    remote.SetAppID("your_appid")
    v := viper.New()
    v.SetConfigType("prop") // 根据namespace实际格式设置对应type
    err := v.AddRemoteProvider("apollo", "your_apollo_endpoint", "your_apollo_namespace")
    // error handle...
    err = v.ReadRemoteConfig()
    // error handle...

    // 直接反序列化到结构体中
    var conf Config
    err = v.Unmarshal(&conf)
    // error handle...
    fmt.Printf("%+v\n", conf)
    
    // 各种基础类型配置项读取
    fmt.Println("Host:", v.GetString("db.host"))
    fmt.Println("Port:", v.GetInt("db.port"))
    fmt.Println("Timeout:", v.GetDuration("db.timeout"))
    
    // 获取所有key，所有配置
    fmt.Println("AllKeys", v.AllKeys(), "AllSettings",  v.AllSettings())
}
```

如果碰到panic: codecgen version mismatch: current: 8, need 10这中错误，详情请见[issue](https://github.com/shima-park/agollo/issues/14)  
解决办法是将etcd升级到3.3.13: 
```
// 使用go module管理依赖包，使用如下命令更新到此版本，或者更高版本
go get github.com/coreos/etcd@v3.3.13+incompatible
```  

## License

The project is licensed under the [Apache 2 license](https://github.com/shima-park/agollo/blob/master/LICENSE).
