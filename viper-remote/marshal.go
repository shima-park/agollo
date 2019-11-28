package remote

import (
	"bytes"
	"fmt"

	"github.com/magiconair/properties"
)

func marshalProperties(c map[string]interface{}) ([]byte, error) {
	p := properties.NewProperties()
	for key, val := range c {
		_, _, err := p.Set(key, fmt.Sprint(val))
		if err != nil {
			return nil, err
		}
	}
	buff := bytes.NewBuffer(nil)
	_, err := p.WriteComment(buff, "#", properties.UTF8)
	if err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}
