package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Pauser(t *testing.T) {
	cases := []struct {
		pause    bool
		expected bool
	}{
		{
			true,
			true,
		},
		{
			false,
			false,
		},
	}

	pauser := NewPauser()
	assert.Equal(t, false, pauser.Value())

	for _, tc := range cases {
		if tc.pause {
			pauser.Pause()
		} else {
			pauser.UnPause()
		}

		assert.Equal(t, tc.expected, pauser.Value())
	}
}
