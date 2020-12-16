package agollo

type ApolloClientOption func(*apolloClient)

func WithDoer(d Doer) ApolloClientOption {
	return func(a *apolloClient) {
		a.Doer = d
	}
}

func WithIP(ip string) ApolloClientOption {
	return func(a *apolloClient) {
		a.IP = ip
	}
}

func WithConfigType(configType string) ApolloClientOption {
	return func(a *apolloClient) {
		a.ConfigType = configType
	}
}

func WithAccessKey(accessKey string) ApolloClientOption {
	return func(a *apolloClient) {
		a.AccessKey = accessKey
	}
}

func WithSignatureFunc(sf SignatureFunc) ApolloClientOption {
	return func(a *apolloClient) {
		a.SignatureFunc = sf
	}
}
