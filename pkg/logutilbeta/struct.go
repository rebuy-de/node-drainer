// Should get merged into sdk, soon.
package logutilbeta

import (
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
)

func FromStruct(s interface{}) logrus.Fields {
	fields := logrus.Fields{}
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "logfield",
		Result:  &fields,
	})
	if err != nil {
		return logrus.Fields{"logfield-error": err}
	}

	err = dec.Decode(s)
	if err != nil {
		return logrus.Fields{"logfield-error": err}
	}

	return fields
}
