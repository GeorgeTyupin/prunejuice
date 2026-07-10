package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that unmarshals from a Go duration string
// ("5m", "30s", "1h30m") or a bare number of seconds. yaml.v3 has no native
// duration support, so this small wrapper provides it.
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		parsed, perr := time.ParseDuration(s)
		if perr != nil {
			return fmt.Errorf("config: invalid duration %q: %w", s, perr)
		}
		d.Duration = parsed
		return nil
	}

	var secs int64
	if err := value.Decode(&secs); err == nil {
		d.Duration = time.Duration(secs) * time.Second
		return nil
	}

	return fmt.Errorf("config: duration must be a string like \"5m\" or a number of seconds, got %q", value.Value)
}

// MarshalYAML implements yaml.Marshaler, emitting the canonical string form.
func (d Duration) MarshalYAML() (any, error) {
	return d.Duration.String(), nil
}
