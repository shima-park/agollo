# Agollo - Go Client for Apollo
================

# agollo
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

##### 简单示例
```
func main(){
    a, err := agollo.New("localhost:8080", "your_appid", agollo.AutoFetchOnCacheMiss())
	if err != nil {
		panic(err)
	}

	fmt.Println(
		a.Get("foo"),                // 获取your_appid下namespace为application中配置项foo的value
		a.GetNameSpace("test.json"), // 获取your_appid下namespace为test.json的所有配置项
	)
}
```
##### 详细特性展示
请将example/sample下app.properties修改为你本地或者测试的apollo配置。
```
func main() {
	// 通过默认根目录下的app.properties初始化agollo
	err := agollo.InitWithDefaultConfigFile(
		agollo.WithLogger(agollo.NewLogger(agollo.LoggerWriter(os.Stdout))), // 打印日志信息
		agollo.PreloadNamespaces("TEST.Namespace"),                          // 预先加载的namespace列表，如果是通过配置启动，会在app.properties配置的基础上追加
		agollo.AutoFetchOnCacheMiss(),                                       // 在配置未找到时，去apollo的带缓存的获取配置接口，获取配置
		agollo.FailTolerantOnBackupExists(),                                 // 在连接apollo失败时，如果在配置的目录下存在.agollo备份配置，会读取备份在服务器无法连接的情况下
	)
	if err != nil {
		panic(err)
	}

	/*
		通过指定配置文件地址的方式初始化
		agollo.InitWithConfigFile(configFilePath string, opts ....Option)

		参数形式初始化agollo的方式，适合二次封装
		(1)使用默认的内置对象，用来直接使用包的静态方法
		agollo.Init(
			"localhost:8080",
			"AppTest",
		        opts...,
		)
		(2)不使用包的静态方法，或者需要新建接口对象
		newAgollo, err := agollo.New(
			"localhost:8080",
			"AppTest",
		        opts...,
			)
	*/

	// 获取默认配置中cluster=default namespace=application key=Name的值
	fmt.Println("Name:", agollo.Get("Name"))

	// 获取默认配置中cluster=default namespace=application key=YourConfigKey的值，提供默认值返回
	fmt.Println("YourConfigKey:", agollo.Get("YourConfigKey", agollo.WithDefault("YourDefaultValue")))

	// 获取默认配置中cluster=default namespace=Test.Namespace key=YourConfigKey2的值，提供默认值返回
	fmt.Println("YourConfigKey2:", agollo.Get("YourConfigKey2", agollo.WithDefault("YourDefaultValue"), agollo.WithNamespace("YourNamespace")))

	// 获取namespace下的所有配置项
	fmt.Println("Configuration of the namespace:", agollo.GetNameSpace("application"))

	// TEST.Namespace1是非预加载的namespace
	// agollo初始化是带上agollo.AutoFetchOnCacheMiss()可选项的话
	// 陪到非预加载的namespace，会去apollo缓存接口获取配置
	// 未配置的话会返回空或者传入的默认值选项
	fmt.Println(agollo.Get("Name", agollo.WithDefault("foo"), agollo.WithNamespace("TEST.Namespace1")))

	// 如果想监听并同步服务器配置变化，启动apollo长轮训
	// 返回一个期间发生错误的error channel,按照需要去处理
	errorCh := agollo.Start()

	// 监听apollo配置更改事件
	// 返回namespace和其变化前后的配置,以及可能出现的error
	watchCh := agollo.Watch()

	go func() {
		for {
			select {
			case err := <-errorCh:
				fmt.Println("Error:", err)
			case update := <-watchCh:
				fmt.Println("Apollo Update:", update)
			}
		}
	}()

	select {}
}
```

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
    // ... error handle
    err = v.ReadRemoteConfig()
    // ... error handle

    // 直接反序列化到结构体中
    var conf Config
    err = v.Unmarshal(&conf)
    // ... error handle
    fmt.Printf("%+v\n", conf)
    
    // 各种基础类型配置项读取
    fmt.Println("Host:", v.GetString("db.host"))
    fmt.Println("Port:", v.GetInt("db.port"))
    fmt.Println("Timeout:", v.GetDuration("db.timeout"))
    
    // 获取所有key，所有配置
    fmt.Println("AllKeys", v.AllKeys(), "AllSettings",  v.AllSettings())
}
```

## License

The project is licensed under the [Apache 2 license](https://github.com/shima-park/agollo/blob/master/LICENSE).
