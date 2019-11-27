package agollo

import (
	"sync"
	"testing"
	"time"

	"gopkg.in/go-playground/assert.v1"
)

func TestAutoFetchBalancer(t *testing.T) {
	refreshIntervalInSecond := time.Second * 2

	expected := []ConfigServer{
		ConfigServer{
			AppName:     "APOLLO-CONFIGSERVICE",
			InstanceID:  "localhost:apollo-configservice:8080",
			HomePageURL: "http://127.0.0.1:8080",
		},
	}

	var wg sync.WaitGroup
	go func() {
		<-time.After(refreshIntervalInSecond)

		expected = append(expected, ConfigServer{
			AppName:     "APOLLO-CONFIGSERVICE",
			InstanceID:  "localhost:apollo-configservice:8081",
			HomePageURL: "http://127.0.0.1:8081",
		})

		wg.Done()
	}()

	getConfigServers := func(metaServerURL, appID string) (int, []ConfigServer, error) {
		return 200, expected, nil
	}

	b, err := NewAutoFetchBalancer("", "", getConfigServers, refreshIntervalInSecond, NewLogger())
	if err != nil {
		t.Fatal(err)
	}

	actual, err := b.Select()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expected[0].HomePageURL, actual)

	wg.Wait()

	for i := 0; i < 10; i++ {
		actual, err := b.Select()
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, expected[i%len(expected)].HomePageURL, actual)
	}
}

func TestRoundRobin(t *testing.T) {
	expected := []string{
		"http://127.0.0.1:8080/",
		"http://127.0.0.1:8081/",
	}

	lb := NewRoundRobin(expected)

	for i := 0; i < 10; i++ {
		actual, err := lb.Select()
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, expected[i%len(expected)], actual)
	}

}
