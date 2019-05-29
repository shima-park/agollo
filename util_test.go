package agollo

import "testing"

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{
			"localhost",
			"http://localhost",
		},
		{
			"http://localhost",
			"http://localhost",
		},
		{
			"https://localhost",
			"https://localhost",
		},
	}

	for i, test := range tests {
		t.Logf("run test (%v): %v", i, test.url)

		actual := normalizeURL(test.url)
		if actual != test.expected {
			t.Errorf("  should be equal (expected=%v, actual=%v)", test.expected, actual)
		}
	}
}
