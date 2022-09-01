package agollo

import "testing"

func Test_getPathWithQuery(t *testing.T) {
	type args struct {
		uri string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "",
			args: args{
				uri: "http://apollo.meta/configsvc-dev/services/config?id=1",
			},
			want: "/configsvc-dev/services/config?id=1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getPathWithQuery(tt.args.uri); got != tt.want {
				t.Errorf("getPathWithQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}
