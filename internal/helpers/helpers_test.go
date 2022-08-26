package helpers

import (
	"testing"
)

func TestMapsAreEqual(t *testing.T) {
	type args struct {
		currentMap map[string]string
		newMap     map[string]string
	}

	// nolint:govet // struct field ordering is fine as is for tests
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"nil maps are equal",
			args{
				nil,
				nil,
			},
			true,
		},
		{
			"current differs",
			args{
				map[string]string{"foo": "bar"},
				nil,
			},
			false,
		},
		{
			"is equal, key orderering differs",
			args{
				map[string]string{"test": "lala", "foo": "bar"},
				map[string]string{"foo": "bar", "test": "lala"},
			},
			true,
		},
		{
			"key exists without value, maps differ",
			args{
				map[string]string{"test": "lala", "foo": "bar"},
				map[string]string{"foo": "bar", "test": ""},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapsAreEqual(tt.args.currentMap, tt.args.newMap); got != tt.want {
				t.Errorf("MapsAreEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
