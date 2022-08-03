# Agollo - Go Client for Apollo

[![Build Status](https://travis-ci.org/shima-park/agollo.svg?branch=master)](https://travis-ci.org/shima-park/agollo)
[![Go Report Card](https://goreportcard.com/badge/github.com/shima-park/agollo)](https://goreportcard.com/report/github.com/shima-park/agollo)
[![GolangCI](https://golangci.com/badges/github.com/shima-park/agollo.svg)](https://golangci.com)
[![codebeat badge](https://codebeat.co/badges/bc2009d6-84f1-4f11-803e-fc571a12a1c0)](https://codebeat.co/projects/github-com-shima-park-agollo-master)
[![golang](https://img.shields.io/badge/Language-Go-green.svg?style=flat)](https://golang.org)
[![GoDoc](http://godoc.org/github.com/shima-park/agollo?status.svg)](http://godoc.org/github.com/shima-park/agollo)
[![GitHub release](https://img.shields.io/github/release/shima-park/agollo.svg)](https://github.com/shima-park/agollo/releases)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

携程Apollo Golang版客户端

针对[apollo openapi](https://github.com/shima-park/apollo-openapi)的golang 客户端封装

## 快速开始
### 获取安装
```
go get -u github.com/shima-park/agollo
```

## Features
* 实时同步配置,配置改动监听
* 配置文件容灾
* 支持多namespace, cluster
* 客户端SLB
* 提供Viper配置库的apollo插件
* 支持通过 ACCESSKEY_SECRET 来实现 client 安全访问
* 支持自定义签名认证

## 示例

### 读取配置
此示例场景适用于程序启动时读取一次。不会额外启动goroutine同步配置
```
package main

import (
	"fmt"

	"github.com/shima-park/agollo"
)

func main() {
	a, err := agollo.New("localhost:8080", "your_appid", agollo.AutoFetchOnCacheMiss())
	if err != nil {
		panic(err)
	}

	fmt.Println(
		// 默认读取Namespace：application下key: foo的value
		a.Get("foo"),

		// 获取namespace为test.json的所有配置项
		a.GetNameSpace("test.json"),

		// 当key：foo不存在时，提供一个默认值bar
		a.Get("foo", agollo.WithDefault("bar")),

		// 读取Namespace为other_namespace, key: foo的value
		a.Get("foo", agollo.WithNamespace("other_namespace")),
	)
}
```

### 实时同步配置
启动一个goroutine实时同步配置, errorCh返回notifications/v2非httpcode(200)的错误信息
```
a, err := agollo.New("localhost:8080", "your_appid", agollo.PreloadNamespaces("application", "test.json"))
// error handle...

errorCh := a.Start()  // Start后会启动goroutine监听变化，并更新agollo对象内的配置cache
// 或者忽略错误处理直接 a.Start()
```

### 配置监听
监听所有namespace配置变更事件
```
a, err := agollo.New("localhost:8080", "your_appid", agollo.PreloadNamespaces("application", "test.json"))
// error handle...

errorCh := a.Start()  // Start后会启动goroutine监听变化，并更新agollo对象内的配置cache
// 或者忽略错误处理直接 a.Start()

watchCh := a.Watch()

for{
	select{
	case err := <- errorCh:
		// handle error
	case resp := <-watchCh:
		fmt.Println(
			"Namespace:", resp.Namespace,
			"OldValue:", resp.OldValue,
			"NewValue:", resp.NewValue,
			"Error:", resp.Error,
		)
	}
}
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
a.Get("foo", agollo.WithNamespace("Namespace_A")) // 从Namespace_A中获取配置项foo
a.Get("foo", agollo.WithNamespace("Namespace_B")) // 从Namespace_B中获取配置项foo
// ...
```

或者初始化时增加agollo.PreloadNamespaces("Namespace_A", "Namespace_B", ...)预加载这几个Namespace的配置
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

### 客户端SLB
客户端通过MetaServer进行动态SLB的启用逻辑：

```
//方式1:
    // 使用者主动增加配置项agollo.EnableSLB(true)
    a, err := agollo.New("localhost:8080", "your_appid", agollo.EnableSLB(true))


//方式2:
    // (客户端显示传递的configServerURL) 和 (环境变量中的APOLLO_CONFIGSERVICE) 都为空值
    // export APOLLO_CONFIGSERVICE=""
    // 此方式必须设置 export APOLLO_META="your meta_server address"
    a, err := agollo.New("", "your_appid")
```

客户端静态SLB(现在支持","分割的多个configServer地址列表):

```
//方式1:
    // 直接传入","分割的多个configServer地址列表
    a, err := agollo.New("localhost:8080,localhost:8081,localhost:8082", "your_appid")

//方式2:
    // 在环境变量中APOLLO_CONFIGSERVICE设置","分割的多个configServer地址列表
    // export APOLLO_CONFIGSERVICE="localhost:8080,localhost:8081,localhost:8082"
    a, err := agollo.New("", "your_appid")
```

SLB更新间隔默认是60s和官方java sdk保持一致，可以通过agollo.ConfigServerRefreshIntervalInSecond(time.Second * 90)来修改
```
    a, err := agollo.New("localhost:8080", "your_appid",
        agollo.EnableSLB(true),
        agollo.ConfigServerRefreshIntervalInSecond(time.Second * 90),
    )
```

! SLB的MetaServer地址来源(用来调用接口获取configServer列表)，取下列表中非空的一项:
1. 用户显示传递的configServerURL
2. 环境变量中的APOLLO_META

! SLB的默认采用的算法是RoundRobin

### 初始化方式

三种package级别初始化，影响默认对象和package提供的静态方法。适用于不做对象传递，单一AppID的场景
```
// 读取当前目录下app.properties，适用于原始apollo定义的读取固定配置文件同学
agollo.InitWithDefaultConfigFile(opts ...Option) error

agollo.Init(configServerURL, appID string, opts ...Option) (err error)

agollo.InitWithConfigFile(configFilePath string, opts ...Option) (err error)
```

两种新建对象初始化方法。返回独立的Agollo接口对象。互相之间不会影响，适用于多AppID，Cluser, ConfigServer配置读取
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

### viper配置同步
基于轮训的配置同步
```
    remote.SetAppID("your_appid")
    v := viper.New()
    v.SetConfigType("prop")
    err := v.AddRemoteProvider("apollo", "your_apollo_endpoint", "your_apollo_namespace")
    // error handle...
    err = v.ReadRemoteConfig()
    // error handle...

    for {
	time.Sleep(10 * time.Second)

	err := v.WatchRemoteConfig() // 每次调用该方法，会从apollo缓存接口获取一次配置，并更新viper
	if err != nil {
		panic(err)
	}

	fmt.Println("AllSettings:", v.AllSettings())
     }
```
基于事件监听配置同步
```
    remote.SetAppID("your_appid")
    v := viper.New()
    v.SetConfigType("prop")
    err := v.AddRemoteProvider("apollo", "your_apollo_endpoint", "your_apollo_namespace")
    // error handle...
    err = v.ReadRemoteConfig()
    // error handle...

    v.WatchRemoteConfigOnChannel() // 启动一个goroutine来同步配置更改

    for {
	time.Sleep(1 * time.Second)
	fmt.Println("AllSettings:", v.AllSettings())
     }
```

## License

The project is licensed under the [Apache 2 license](https://github.com/shima-park/agollo/blob/master/LICENSE).
