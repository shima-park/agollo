package agollo

import (
	"errors"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var (
	defaultRefreshIntervalInSecond = time.Second * 60
	defaultMetaURL                 = "http://apollo.meta"
	ErrNoConfigServerAvailable     = errors.New("no config server availbale")
)

type Balancer interface {
	Select() (string, error)
	Stop()
}

type autoFetchBalancer struct {
	appID             string
	getConfigServers  GetConfigServersFunc
	metaServerAddress string

	logger Logger

	mu sync.RWMutex
	b  Balancer

	stopCh chan struct{}
}

type GetConfigServersFunc func(metaServerURL, appID string) (int, []ConfigServer, error)

func NewAutoFetchBalancer(configServerURL, appID string, getConfigServers GetConfigServersFunc,
	refreshIntervalInSecond time.Duration, logger Logger) (Balancer, error) {

	if refreshIntervalInSecond <= time.Duration(0) {
		refreshIntervalInSecond = defaultRefreshIntervalInSecond
	}

	b := &autoFetchBalancer{
		appID:             appID,
		getConfigServers:  getConfigServers,
		metaServerAddress: getMetaServerAddress(configServerURL), // Meta Server只是一个逻辑角色，在部署时和Config Service是在一个JVM进程中的，所以IP、端口和Config Service一致
		logger:            logger,
		stopCh:            make(chan struct{}),
		b:                 NewRoundRobin([]string{configServerURL}),
	}

	err := b.updateConfigServices()
	if err != nil {
		return nil, err
	}

	go func() {
		ticker := time.NewTicker(refreshIntervalInSecond)
		defer ticker.Stop()

		for {
			select {
			case <-b.stopCh:
				return
			case <-ticker.C:
				_ = b.updateConfigServices()
			}
		}
	}()

	return b, nil
}

/*
参考了java客户端实现
目前实现方式:
0. 客户端显式传入ConfigServerURL
1. 读取APOLLO_META环境变量
2. 默认如果没有提供meta服务地址默认使用(http://apollo.meta)

未实现:
读取properties的逻辑
https://github.com/ctripcorp/apollo/blob/7545bd3cd7d4b996d7cda50f53cd4aa8b045a2bb/apollo-core/src/main/java/com/ctrip/framework/apollo/core/MetaDomainConsts.java#L27
*/
func getMetaServerAddress(configServerURL string) string {
	var urls []string
	for _, url := range []string{
		configServerURL,
		os.Getenv("APOLLO_META"),
	} {
		if url != "" {
			urls = splitCommaSeparatedURL(url)
			break
		}
	}

	if len(urls) > 0 {
		return normalizeURL(urls[rand.Intn(len(urls))])
	}

	return defaultMetaURL
}

func (b *autoFetchBalancer) updateConfigServices() error {
	css, err := b.getConfigServices()
	if err != nil {
		return err
	}

	var urls []string
	for _, url := range css {
		//check whether /services/config is accessible
		status, _, err := b.getConfigServers(url, b.appID)
		if err != nil {
			continue
		}

		// select the first available meta server
		// https://github.com/ctripcorp/apollo/blob/7545bd3cd7d4b996d7cda50f53cd4aa8b045a2bb/apollo-core/src/main/java/com/ctrip/framework/apollo/core/MetaDomainConsts.java#L166
		// 这里这段逻辑是参考java客户端，直接选了第一个可用的meta server
		if 200 <= status && status <= 399 {
			urls = append(urls, url)
			break
		}
	}

	if len(urls) == 0 {
		return nil
	}

	b.mu.Lock()
	b.b = NewRoundRobin(css)
	b.mu.Unlock()

	return nil
}

func (b *autoFetchBalancer) getConfigServices() ([]string, error) {
	_, css, err := b.getConfigServers(b.metaServerAddress, b.appID)
	if err != nil {
		b.logger.Log(
			"[Agollo]", "",
			"AppID", b.appID,
			"MetaServerAddress", b.metaServerAddress,
			"Error", err,
		)
		return nil, err
	}

	var urls []string
	for _, cs := range css {
		urls = append(urls, normalizeURL(cs.HomePageURL))
	}

	return urls, nil
}

func (b *autoFetchBalancer) Select() (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.b.Select()
}

func (b *autoFetchBalancer) Stop() {
	close(b.stopCh)
}

type roundRobin struct {
	ss []string
	c  uint64
}

func NewRoundRobin(ss []string) Balancer {
	return &roundRobin{
		ss: ss,
		c:  0,
	}
}

func (rr *roundRobin) Select() (string, error) {
	if len(rr.ss) <= 0 {
		return "", ErrNoConfigServerAvailable
	}

	old := atomic.AddUint64(&rr.c, 1) - 1
	idx := old % uint64(len(rr.ss))
	return rr.ss[idx], nil
}

func (rr *roundRobin) Stop() {

}
