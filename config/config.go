package config

import (
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// LoadConfig loads configuration from environment variables using struct tags
func LoadConfig[T any](envFile ...string) T {
	if len(envFile) > 0 {
		godotenv.Load(envFile[0])
	} else {
		godotenv.Load()
	}

	var config T
	v := reflect.ValueOf(&config).Elem()
	t := reflect.TypeOf(config)

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		envTag := fieldType.Tag.Get("env")
		defaultTag := fieldType.Tag.Get("default")

		if envTag == "" {
			continue
		}

		envValue := os.Getenv(envTag)
		if envValue == "" {
			envValue = defaultTag
		}

		if envValue != "" {
			setFieldValue(field, envValue)
		}
	}

	return config
}

// setFieldValue sets field value from string based on field type
func setFieldValue(field reflect.Value, value string) {
	if !field.CanSet() {
		return
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int64:
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			field.SetInt(i)
		}
	case reflect.Bool:
		if b, err := strconv.ParseBool(value); err == nil {
			field.SetBool(b)
		}
	case reflect.TypeOf(time.Duration(0)).Kind():
		if d, err := time.ParseDuration(value); err == nil {
			field.Set(reflect.ValueOf(d))
		}
	}
}
