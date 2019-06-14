package remote

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/hcl/hcl/printer"

	"github.com/hashicorp/hcl"
	"github.com/magiconair/properties"
	toml "github.com/pelletier/go-toml"
	yaml "gopkg.in/yaml.v2"
)

// ConfigMarshalError happens when failing to marshal the configuration.
type ConfigMarshalError struct {
	err error
}

// Error returns the formatted configuration error.
func (e ConfigMarshalError) Error() string {
	return fmt.Sprintf("While marshaling config: %s", e.err.Error())
}

func MarshalWriter(w io.Writer, c map[string]interface{}, configType string) error {
	switch configType {
	case "json":
		b, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return ConfigMarshalError{err}
		}
		_, err = w.Write(b)
		if err != nil {
			return ConfigMarshalError{err}
		}

	case "hcl":
		b, err := json.Marshal(c)
		if err != nil {
			return ConfigMarshalError{err}
		}
		ast, err := hcl.Parse(string(b))
		if err != nil {
			return ConfigMarshalError{err}
		}
		err = printer.Fprint(w, ast.Node)
		if err != nil {
			return ConfigMarshalError{err}
		}

	case "prop", "props", "properties":
		p := properties.NewProperties()
		for key, val := range c {
			_, _, err := p.Set(key, fmt.Sprint(val))
			if err != nil {
				return ConfigMarshalError{err}
			}
		}
		_, err := p.WriteComment(w, "#", properties.UTF8)
		if err != nil {
			return ConfigMarshalError{err}
		}

	case "toml":
		t, err := toml.TreeFromMap(c)
		if err != nil {
			return ConfigMarshalError{err}
		}
		if _, err := t.WriteTo(w); err != nil {
			return ConfigMarshalError{err}
		}

	case "yaml", "yml":
		b, err := yaml.Marshal(c)
		if err != nil {
			return ConfigMarshalError{err}
		}
		if _, err = w.Write(b); err != nil {
			return ConfigMarshalError{err}
		}
	}
	return nil
}
