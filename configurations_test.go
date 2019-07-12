package agollo

import "testing"

func TestConfigurationsDifferent(t *testing.T) {
	tests := []struct {
		old     Configurations
		new     Configurations
		changes []Change
	}{
		{
			Configurations{
				"name":    "foo",
				"age":     18,
				"balance": 101.2,
			},

			Configurations{
				"name":   "foo",
				"age":    19,
				"height": 1.82,
			},
			[]Change{
				Change{ChangeTypeUpdate, "age", 19},
				Change{ChangeTypeDelete, "balance", 101.2},
				Change{ChangeTypeAdd, "height", 1.82},
			},
		},
	}

	for _, test := range tests {
		changes := test.old.Different(test.new)

		if len(changes) != len(test.changes) {
			t.Errorf("should be equal (expected=%v, actual=%v)", test.changes, len(changes))
		}
		for i, actual := range changes {
			expected := test.changes[i]
			if actual.Type != expected.Type &&
				actual.Key != expected.Key &&
				actual.Value != expected.Value {
				t.Errorf("should be equal (expected=%v, actual=%v)", expected, actual)
			}
		}
	}
}
