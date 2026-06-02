package utils

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/creasty/defaults"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

const DefaultFileType = "yaml"

type ConfigItem interface {
	UnmarshalConfigItem(val string) error
}

func LoadConfig(path string, configObject interface{}) {
	LoadConfigWithType(path, getFileType(path), configObject)
}

func LoadConfigWithType(path string, fileType string, configObject interface{}) {
	if fileType == "" {
		viper.SetConfigType(DefaultFileType)
	} else {
		viper.SetConfigType(fileType)
	}

	if path != "" {
		plan, err := os.ReadFile(path)
		if err != nil {
			panic(errors.Wrap(err, fmt.Sprintf("cannot read file %s", path)))
		}

		if err = viper.ReadConfig(bytes.NewBuffer(plan)); err != nil {
			panic(errors.Wrap(err, fmt.Sprintf("cannot read content of file %s", path)))
		}
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))
	viper.AutomaticEnv()

	if err := viper.Unmarshal(configObject, viperDecodeHook()); err != nil {
		panic(errors.Wrap(err, fmt.Sprintf("cannot parse file %s to object", path)))
	}

	if err := defaults.Set(configObject); err != nil {
		panic(errors.Wrap(err, fmt.Sprintf("cannot set default values from file %s to object", path)))
	}
}

func GetStringMapFixed(key string) map[string]string {
	out := make(map[string]string)
	for _, k := range os.Environ() {
		if strings.HasPrefix(k, key) {
			parts := strings.Split(k, "=")
			keys := strings.Split(parts[0], "__")
			out[parts[1]] = keys[1]
		}
	}
	return out
}

func getFileType(path string) string {
	ext := filepath.Ext(path)
	return ext[1:]
}

func viperDecodeHook() viper.DecoderConfigOption {
	return viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		configItemHookFunc(),
	))
}

func configItemHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		item, ok := reflect.New(t).Interface().(ConfigItem)
		if !ok {
			return data, nil
		}

		input := fmt.Sprintf("%v", data)
		err := item.UnmarshalConfigItem(input)
		return item, err
	}
}
