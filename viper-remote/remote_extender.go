package remote

import (
	"bytes"
	"github.com/spf13/viper"
)
type Extender struct {
	// provider information
	provider      string
	endpoint      string
	path          string
	secretKeyring string
	// viper
	viper 		  *viper.Viper
}

func (rp *Extender) Provider() string {
	return rp.provider
}

func (rp *Extender) Endpoint() string {
	return rp.endpoint
}

func (rp *Extender) Path() string {
	return rp.path
}

func (rp *Extender) SecretKeyring() string {
	return rp.secretKeyring
}

// ApolloConfig Apollo Config
type ApolloConfig struct {
	// apollo endpoint or url
	Endpoint   string
	// apollo appid
	AppID      string
	// config file type ,such as prop or yaml
	ConfigType string
	// apollo namespace
	Namespace  string
}

// NewApolloProvider new apollo viper remote provider by apollo config
func NewApolloProvider(config ApolloConfig)  (*Extender,error) {
	return newApolloProvider(config.Endpoint,config.AppID,config.ConfigType,config.Namespace)
}

// NewApolloProvider new apollo viper remote provider
func newApolloProvider(endpoint string,appid string,configType string,namespace string) (*Extender,error) {
	SetAppID(appid)
	v := viper.New()
	v.SetConfigType(configType)
	err := v.AddRemoteProvider("apollo", endpoint, namespace)
	return &Extender{provider: "apollo", endpoint: endpoint, path: namespace, secretKeyring: "" , viper: v},err
}

// GetViper get viper instance
func (rp *Extender) GetViper() *viper.Viper {
	return rp.viper
}

// WatchRemoteConfigOnChannel watch remote config changed on notify
func (rp *Extender) WatchRemoteConfigOnChannel() <-chan bool {
	updater := make(chan bool)
	respChan, _ := viper.RemoteConfig.WatchChannel(rp)
	go func(rc <-chan *viper.RemoteResponse) {
		for {
			b := <-rc
			reader := bytes.NewReader(b.Value)
			_ = rp.viper.ReadConfig(reader)
			// configuration on changed
			updater <- true
		}
	}(respChan)

	return updater
}

