package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
	crypt "github.com/xordataexchange/crypt/config"
	"github.com/shima-park/agollo"
)

var (
	ErrUnsupportedProvider = errors.New("This configuration manager is not supported")

	_ viperConfigManager = apolloConfigManager{}
	// apollod的appid
	appID string
	// 默认为json，因为远程读出来的是数据流，viper并不知道是什么类型的配置文件，所以需要设置配置文件类型，来进行反序列化
	defaultConfigType = "properties"
	// getConfigManager方法每次返回新对象导致缓存无效，
	// 这里通过endpoint作为key复一个对象
	// key: endpoint value: agollo.Agollo
	agolloMap sync.Map
)

func SetAppID(appid string) {
	appID = appid
}

func SetConfigType(ct string) {
	defaultConfigType = ct
}

type viperConfigManager interface {
	Get(key string) ([]byte, error)
	Watch(key string, stop chan bool) <-chan *viper.RemoteResponse
}

type apolloConfigManager struct {
	keystore []byte
	agollo   agollo.Agollo
}

func newApolloConfigManager(appid, endpoint string) (*apolloConfigManager, error) {
	if appID == "" {
		return nil, errors.New("The appid is not set")
	}

	ag, err := newAgollo(appid, endpoint)
	if err != nil {
		return nil, err
	}

	return &apolloConfigManager{
		agollo: ag,
	}, nil

}

func newAgollo(appid, endpoint string) (agollo.Agollo, error) {
	i, found := agolloMap.Load(endpoint)
	if !found {
		ag, err := agollo.New(
			endpoint,
			appid,
			agollo.AutoFetchOnCacheMiss(),
			agollo.FailTolerantOnBackupExists(),
		)
		if err != nil {
			return nil, err
		}

		// TODO 监听apollo配置变化，同步配置，不关闭也没问题?
		ag.Start()

		agolloMap.Store(endpoint, ag)

		return ag, nil
	}
	return i.(agollo.Agollo), nil
}

func (cm apolloConfigManager) Get(namespace string) ([]byte, error) {
	configs := cm.agollo.GetNameSpace(namespace)
	configType := getConfigType(namespace)
	buff := bytes.NewBuffer(nil)
	settings := map[string]interface{}{}

	switch configType {
	case "json", "yml", "yaml", "xml":
		content := configs["content"].(string)
		err := UnmarshalReader(strings.NewReader(content), settings, configType)
		if err != nil {
			return nil, err
		}
	case "properties":
		settings = configs
	}
	err := MarshalWriter(buff, settings, configType)
	return buff.Bytes(), err
}

// 当viper使用远端配置中心时，例如consul, etcd. apollo
// 通过Get获取map[string]interface{}，并尝试递归每个value
// 当存储为原始的string时，对应数组等类型会获取失败
// 所以这里尝试将每个json string还原成go的类型再存储给viper的配置
func resolveValueToGoInterface(configType string, val interface{}) interface{} {
	switch configType {
	case "json":
		var i interface{}
		err := json.Unmarshal([]byte(val.(string)), &i)
		if err == nil {
			// 支持的类型
			// bool "true"
			// float64 "1.11"
			// string "\"aaa\""
			// []interface {} "[\"./log/\", \"stdout\"]"
			// map[string]interface {} "{\"a\":\"a\"}"
			val = i
		}
	}
	return val
}

// 将a.b.c这种key切分成数组[a,b,c]，使用该数组的每个值生成一个嵌套的map
// 最后将值存储到最终的map中
func setMap(source map[string]interface{}, path []string, val interface{}) {
	if len(path) == 0 {
		return
	}

	next, ok := source[path[0]]
	if ok {
		if len(path) == 1 {
			next.(map[string]interface{})[path[0]] = val
			return
		}
		setMap(next.(map[string]interface{}), path[1:], val)
	} else {
		if len(path) == 1 {
			source[path[0]] = val
			return
		}
		source[path[0]] = map[string]interface{}{}
		setMap(source[path[0]].(map[string]interface{}), path[1:], val)
	}
	return
}

func (cm apolloConfigManager) Watch(namespace string, stop chan bool) <-chan *viper.RemoteResponse {
	resp := make(chan *viper.RemoteResponse, 0)
	backendResp := cm.agollo.WatchNamespace(namespace, stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case r := <-backendResp:
				if r.Error != nil {
					resp <- &viper.RemoteResponse{
						Value: nil,
						Error: r.Error,
					}
					continue
				}

				buff := bytes.NewBuffer(nil)

				// TODO 如何推断或者获取当前viper当前的config type
				// 默认用json
				err := MarshalWriter(buff, r.NewValue, getConfigType(namespace))
				value := buff.Bytes()
				resp <- &viper.RemoteResponse{Value: value, Error: err}
			}
		}
	}()
	return resp
}

type configProvider struct {
}

func (rc configProvider) Get(rp viper.RemoteProvider) (io.Reader, error) {
	cmt, err := getConfigManager(rp)
	if err != nil {
		return nil, err
	}

	var b []byte
	switch cm := cmt.(type) {
	case viperConfigManager:
		b, err = cm.Get(rp.Path())
	case crypt.ConfigManager:
		b, err = cm.Get(rp.Path())
	}

	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

func (rc configProvider) Watch(rp viper.RemoteProvider) (io.Reader, error) {
	cmt, err := getConfigManager(rp)
	if err != nil {
		return nil, err
	}

	var resp []byte
	switch cm := cmt.(type) {
	case viperConfigManager:
		resp, err = cm.Get(rp.Path())
	case crypt.ConfigManager:
		resp, err = cm.Get(rp.Path())
	}

	if err != nil {
		return nil, err
	}

	return bytes.NewReader(resp), nil
}

func (rc configProvider) WatchChannel(rp viper.RemoteProvider) (<-chan *viper.RemoteResponse, chan bool) {
	cmt, err := getConfigManager(rp)
	if err != nil {
		return nil, nil
	}

	switch cm := cmt.(type) {
	case viperConfigManager:
		quitwc := make(chan bool)
		viperResponsCh := cm.Watch(rp.Path(), quitwc)
		return viperResponsCh, quitwc
	default:
		ccm := cm.(crypt.ConfigManager)
		quit := make(chan bool)
		quitwc := make(chan bool)
		viperResponsCh := make(chan *viper.RemoteResponse)
		cryptoResponseCh := ccm.Watch(rp.Path(), quit)
		// need this function to convert the Channel response form crypt.Response to viper.Response
		go func(cr <-chan *crypt.Response, vr chan<- *viper.RemoteResponse, quitwc <-chan bool, quit chan<- bool) {
			for {
				select {
				case <-quitwc:
					quit <- true
					return
				case resp := <-cr:
					vr <- &viper.RemoteResponse{
						Error: resp.Error,
						Value: resp.Value,
					}

				}

			}
		}(cryptoResponseCh, viperResponsCh, quitwc, quit)

		return viperResponsCh, quitwc
	}
}

func getConfigManager(rp viper.RemoteProvider) (interface{}, error) {
	if rp.SecretKeyring() != "" {
		kr, err := os.Open(rp.SecretKeyring())
		defer kr.Close()
		if err != nil {
			return nil, err
		}

		switch rp.Provider() {
		case "etcd":
			return crypt.NewEtcdConfigManager([]string{rp.Endpoint()}, kr)
		case "consul":
			return crypt.NewConsulConfigManager([]string{rp.Endpoint()}, kr)
		case "apollo":
			return nil, errors.New("The Apollo configuration manager is not encrypted")
		default:
			return nil, ErrUnsupportedProvider
		}
	} else {
		switch rp.Provider() {
		case "etcd":
			return crypt.NewStandardEtcdConfigManager([]string{rp.Endpoint()})
		case "consul":
			return crypt.NewStandardConsulConfigManager([]string{rp.Endpoint()})
		case "apollo":
			return newApolloConfigManager(appID, rp.Endpoint())
		default:
			return nil, ErrUnsupportedProvider
		}
	}
}

// 配置文件有多种格式，例如：properties、xml、yml、yaml、json等。同样Namespace也具有这些格式。在Portal UI中可以看到“application”的Namespace上有一个“properties”标签，表明“application”是properties格式的。
// 如果使用Http接口直接调用时，对应的namespace参数需要传入namespace的名字加上后缀名，如datasources.json。
func getConfigType(namespace string) string {
	ext := filepath.Ext(namespace)

	if len(ext) > 1 {
		fileExt := ext[1:]
		// 还是要判断一下碰到，TEST.Namespace1
		// 会把Namespace1作为文件扩展名
		for _, e := range viper.SupportedExts {
			if e == fileExt {
				return fileExt
			}
		}
	}

	return defaultConfigType
}

func init() {
	viper.SupportedRemoteProviders = append(
		viper.SupportedRemoteProviders,
		"apollo",
	)
	viper.RemoteConfig = &configProvider{}
}
