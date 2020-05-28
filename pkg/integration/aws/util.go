package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
)

func ec2tag(instance *ec2.Instance, key string) string {
	for _, tag := range instance.Tags {
		if aws.StringValue(tag.Key) == key {
			return aws.StringValue(tag.Value)
		}
	}

	return ""
}

func logFieldsFromStruct(s interface{}) logrus.Fields {
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
