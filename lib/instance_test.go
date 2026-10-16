package lib

import "testing"

func TestRejectInvalidConfig(t *testing.T) {
	for _, raw := range []string{"null", `{"input":[null]}`, `{"output":[null]}`, "{/*"} {
		i := &instance{}
		if err := i.initConfigFromBytes([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
		if len(i.input) != 0 || len(i.output) != 0 {
			t.Fatal("invalid config mutated instance")
		}
	}
}
