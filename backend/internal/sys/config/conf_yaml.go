package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// strictYAML loads exactly one YAML document and rejects keys that do not map
// to fields in the configuration struct.
type strictYAML struct {
	data []byte
}

// Process implements conf.Parsers.
func (parser strictYAML) Process(_ string, cfg any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(parser.data))
	decoder.KnownFields(true)

	if err := decoder.Decode(cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("decode YAML: %w", err)
	}

	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("decode trailing YAML: %w", err)
	}

	return errors.New("multiple YAML documents are not supported")
}
