package main

import (
	"fmt"
	"time"

	remote "github.com/shima-park/agollo/viper-remote"
	"github.com/spf13/viper"
)

func main() {
	//agollo.Init("192.168.12.15:8080", "AppTest")
	remote.SetAppID("SampleApp")

	app := viper.New()
	test := viper.New()

	// apollo默认的配置文件是properties格式的
	app.SetConfigType("prop")
	test.SetConfigType("prop")

	// 一个namespace相当于一个配置文件
	// 需要不同的viper对象进行读取管理，否则会有key冲突等问题
	app.AddRemoteProvider("apollo", "localhost:8080", "application")
	test.AddRemoteProvider("apollo", "localhost:8080", "TEST.Namespace1")

	app.ReadRemoteConfig()
	test.ReadRemoteConfig()

	fmt.Println("viper.SupportedRemoteProviders:", viper.SupportedRemoteProviders)

	fmt.Println("app.AllSettings:", app.AllSettings())
	fmt.Println("test.AllSettings:", test.AllSettings())

	fmt.Println("Get name in app's namespace(applicatoin):", app.Get("Name"))
	fmt.Println("Get go in test's namespace(TEST.Namespace1):", test.Get("go"))

	isLazySync := true

	if isLazySync {
		// 基于轮训的配置同步
		for {
			time.Sleep(1 * time.Second)

			err := app.WatchRemoteConfig()
			if err != nil {
				panic(err)
			}

			err = test.WatchRemoteConfig()
			if err != nil {
				panic(err)
			}

			fmt.Println("app.AllSettings:", app.AllSettings())
			fmt.Println("test.AllSettings:", test.AllSettings())
		}
	} else {
		// 启动一个goroutine来同步配置更改
		app.WatchRemoteConfigOnChannel()
		test.WatchRemoteConfigOnChannel()
		for {
			time.Sleep(1 * time.Second)
			fmt.Println("app.AllSettings:", app.AllSettings())
			fmt.Println("test.AllSettings:", test.AllSettings())
		}
	}

}
