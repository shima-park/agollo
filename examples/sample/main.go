package main

import (
	"fmt"
	"os"
	"time"

	"github.com/shima-park/agollo"
)

func main() {
	// 通过默认根目录下的app.properties初始化agollo
	err := agollo.InitWithDefaultConfigFile(
		agollo.WithLogger(agollo.NewLogger(agollo.LoggerWriter(os.Stdout))), // 打印日志信息
		agollo.PreloadNamespaces("TEST.Namespace"),                          // 预先加载的namespace列表，如果是通过配置启动，会在app.properties配置的基础上追加
		agollo.AutoFetchOnCacheMiss(),                                       // 在配置未找到时，去apollo的带缓存的获取配置接口，获取配置
		agollo.FailTolerantOnBackupExists(),                                 // 在连接apollo失败时，如果在配置的目录下存在.agollo备份配置，会读取备份在服务器无法连接的情况下

		// apollo 官方签名认证
		// agollo.WithClientOptions(
		// 	agollo.WithAccessKey("your_access_key"),
		// ),

		// 自定义签名认证
		// agollo.WithClientOptions(
		// 	agollo.WithSignatureFunc(
		// 		func(*agollo.SignatureContext) agollo.Header {
		// 			return agollo.Header{
		// 				"Authorization": "basic xxxxxxxx",
		// 			}
		// 		}),
		// ),
	)
	if err != nil {
		panic(err)
	}

	/*
		通过指定配置文件地址的方式初始化
		agollo.InitWithConfigFile(configFilePath string, opts ....Option)

		参数形式初始化agollo的方式，适合二次封装
		agollo.Init(
			"localhost:8080",
			"AppTest",
		        opts...,
		)
	*/

	// 获取默认配置中cluster=default namespace=application key=Name的值
	fmt.Println("timeout:", agollo.Get("timeout"))

	// 获取默认配置中cluster=default namespace=application key=timeout的值，提供默认值返回
	fmt.Println("YourConfigKey:", agollo.Get("YourConfigKey", agollo.WithDefault("YourDefaultValue")))

	// 获取默认配置中cluster=default namespace=Test.Namespace key=timeout的值，提供默认值返回
	fmt.Println("YourConfigKey2:", agollo.Get("YourConfigKey2", agollo.WithDefault("YourDefaultValue"), agollo.WithNamespace("YourNamespace")))

	// 获取namespace下的所有配置项
	fmt.Println("Configuration of the namespace:", agollo.GetNameSpace("application"))

	// TEST.Namespace1是非预加载的namespace
	// agollo初始化是带上agollo.AutoFetchOnCacheMiss()可选项的话
	// 陪到非预加载的namespace，会去apollo缓存接口获取配置
	// 未配置的话会返回空或者传入的默认值选项
	fmt.Println(agollo.Get("timeout", agollo.WithDefault("foo"), agollo.WithNamespace("TEST.Namespace1")))

	// 如果想监听并同步服务器配置变化，启动apollo长轮训
	// 返回一个期间发生错误的error channel,按照需要去处理
	errorCh := agollo.Start()
	defer agollo.Stop()

	// 监听apollo配置更改事件
	// 返回namespace和其变化前后的配置,以及可能出现的error
	watchCh := agollo.Watch()

	stop := make(chan bool)
	watchNamespace := "YourNamespace"
	watchNSCh := agollo.WatchNamespace(watchNamespace, stop)

	appNSCh := agollo.WatchNamespace("application", stop)
	go func() {
		for {
			select {
			case err := <-errorCh:
				fmt.Println("Error:", err)
			case resp := <-watchCh:
				fmt.Println("Watch Apollo:", resp)
			case resp := <-watchNSCh:
				fmt.Println("Watch Namespace", watchNamespace, resp)
			case resp := <-appNSCh:
				fmt.Println("Watch Namespace", "application", resp)
			case <-time.After(time.Second):
				fmt.Println("timeout:", agollo.Get("timeout"))
			}
		}
	}()

	select {}
}
