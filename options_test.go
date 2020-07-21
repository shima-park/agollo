package agollo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOptions(t *testing.T) {
	var (
		configServerURL = "localhost:8080"
		appID           = "SampleApp"
	)
	var tests = []struct {
		Options []Option
		Check   func(Options)
	}{
		{
			[]Option{},
			func(opts Options) {
				assert.Equal(t, appID, opts.AppID)
				assert.Equal(t, defaultCluster, opts.Cluster)
				assert.Equal(t, defaultAutoFetchOnCacheMiss, opts.AutoFetchOnCacheMiss)
				assert.Equal(t, defaultLongPollInterval, opts.LongPollerInterval)
				assert.Equal(t, defaultBackupFile, opts.BackupFile)
				assert.Equal(t, defaultFailTolerantOnBackupExists, opts.FailTolerantOnBackupExists)
				assert.Equal(t, defaultEnableSLB, opts.EnableSLB)
				assert.NotNil(t, opts.Logger)
				assert.NotNil(t, opts.ApolloClient)
				assert.NotNil(t, opts.Balancer)
				assert.Empty(t, opts.PreloadNamespaces)
				assert.Empty(t, opts.DefaultNamespace)
				getOpts := opts.newGetOptions()
				assert.Equal(t, "application", getOpts.Namespace)
				getOpts = opts.newGetOptions(WithNamespace("customize_namespace"))
				assert.Equal(t, "customize_namespace", getOpts.Namespace)
				ac := &apolloClient{}
				ac.Apply(opts.ClientOptions...)
				assert.Empty(t, ac.AccessKey)
			},
		},
		{
			[]Option{
				Cluster("test_cluster"),
				DefaultNamespace("default_namespace"),
				PreloadNamespaces("preload_namespace"),
				AutoFetchOnCacheMiss(),
				LongPollerInterval(time.Second * 30),
				BackupFile("test_backup"),
				FailTolerantOnBackupExists(),
				AccessKey("test_access_key"),
			},
			func(opts Options) {
				assert.Equal(t, "test_cluster", opts.Cluster)
				assert.Equal(t, []string{"preload_namespace", "default_namespace"}, opts.PreloadNamespaces)
				assert.Equal(t, "default_namespace", opts.DefaultNamespace)
				getOpts := opts.newGetOptions()
				assert.Equal(t, "default_namespace", getOpts.Namespace)
				getOpts = opts.newGetOptions(WithNamespace("customize_namespace"))
				assert.Equal(t, "customize_namespace", getOpts.Namespace)
				assert.Equal(t, true, opts.AutoFetchOnCacheMiss)
				assert.Equal(t, time.Second*30, opts.LongPollerInterval)
				assert.Equal(t, "test_backup", opts.BackupFile)
				assert.Equal(t, true, opts.FailTolerantOnBackupExists)
				ac := &apolloClient{}
				ac.Apply(opts.ClientOptions...)
				assert.Equal(t, "test_access_key", ac.AccessKey)
			},
		},
		{
			[]Option{
				EnableSLB(true),
				WithApolloClient(&mockApolloClient{
					getConfigServers: func(metaServerURL, appID string) (i int, servers []ConfigServer, err error) {
						return 200,
							[]ConfigServer{
								ConfigServer{
									AppName:     "test",
									InstanceID:  "test",
									HomePageURL: "http://localhost:8080",
								},
							},
							nil
					},
				}),
			},
			func(opts Options) {
				assert.Equal(t, true, opts.EnableSLB)
			},
		},
	}

	for _, test := range tests {
		opts, err := newOptions(configServerURL, appID, test.Options...)
		if err != nil {
			assert.Nil(t, err)
		}
		test.Check(opts)
	}
}
