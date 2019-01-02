package main

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/shima-park/agollo/viper-remote"
)

func main() {
	//agollo.Init("192.168.12.15:8080", "AppTest")
	remote.SetAppID("AppTest")

	app := viper.New()
	test := viper.New()

	// apollo默认的配置文件是properties格式的
	app.SetConfigType("prop")
	test.SetConfigType("prop")

	// 一个namespace相当于一个配置文件
	// 需要不同的viper对象进行读取管理，否则会有key冲突等问题
	app.AddRemoteProvider("apollo", "192.168.12.15:8080", "application")
	test.AddRemoteProvider("apollo", "192.168.12.15:8080", "TEST.Namespace1")

	app.ReadRemoteConfig()
	test.ReadRemoteConfig()

	fmt.Println("viper.SupportedRemoteProviders:", viper.SupportedRemoteProviders)

	fmt.Println("app.AllKeys:", app.AllKeys())
	fmt.Println("test.AllKeys:", test.AllKeys())

	fmt.Println("Get name in app's namespace(applicatoin):", app.Get("Name"))
	fmt.Println("Get go in test's namespace(TEST.Namespace1):", test.Get("go"))

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

		fmt.Println("Get name in app's namespace(applicatoin):", app.Get("Name"))
		fmt.Println("Get go in test's namespace(TEST.Namespace1):", test.Get("go"))
	}
}
