package basispoints

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigurationUpdateReturnsProtocolError(t *testing.T) {
	source := testSource()
	source["input"] = []any{
		message("user", "continue"),
		object{"type": "configuration_update", "reasoning": object{"effort": "high"}},
	}
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	_, _, err = Prepare(raw, "scope", nil)
	require.ErrorIs(t, err, ErrConfigurationUpdateUnsupported)
	require.EqualError(t, err, "basispoints does not support configuration_update; start a new request with the desired effort")

	source["input"] = []any{message("user", "configuration_update is just text here")}
	raw, err = json.Marshal(source)
	require.NoError(t, err)
	_, _, err = Prepare(raw, "scope", nil)
	require.NoError(t, err)
}
