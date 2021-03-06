package utils

import (
	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
	"os"
	"reflect"
	"strings"
)

const defaultRegion = "eu-de"

func DeleteNotPassParams(params *map[string]interface{}, notPassParams []string) {
	for _, i := range notPassParams {
		delete(*params, i)
	}
}

// merges two interfaces. In cases where a value is defined for both 'overridingInterface' and
// 'inferiorInterface' the value in 'overridingInterface' will take precedence.
func MergeInterfaces(overridingInterface, inferiorInterface interface{}) interface{} {
	switch overriding := overridingInterface.(type) {
	case map[string]interface{}:
		interfaceMap, ok := inferiorInterface.(map[string]interface{})
		if !ok {
			return overriding
		}
		for k, v := range interfaceMap {
			if overridingValue, ok := overriding[k]; ok {
				overriding[k] = MergeInterfaces(overridingValue, v)
			} else {
				overriding[k] = v
			}
		}
	case []interface{}:
		list, ok := inferiorInterface.([]interface{})
		if !ok {
			return overriding
		}
		for i := range list {
			overriding = append(overriding, list[i])
		}
		return overriding
	case nil:
		// mergeClouds(nil, map[string]interface{...}) -> map[string]interface{...}
		v, ok := inferiorInterface.(map[string]interface{})
		if ok {
			return v
		}
	}
	// We don't want to override with empty values
	if reflect.DeepEqual(overridingInterface, nil) || reflect.DeepEqual(reflect.Zero(reflect.TypeOf(overridingInterface)).Interface(), overridingInterface) {
		return inferiorInterface
	} else {
		return overridingInterface
	}
}

func PrependString(item string, slice []string) []string {
	newSize := len(slice) + 1
	result := make([]string, newSize, newSize)
	result[0] = item
	for i, v := range slice {
		result[i+1] = v
	}
	return result
}

func In(item interface{}, slice interface{}) bool {
	for _, it := range slice.([]interface{}) {
		if reflect.DeepEqual(item, it) {
			return true
		}
	}
	return false
}

// GetRegion returns the region that was specified in the auth options. If a
// region was not set it returns value from env OS_REGION_NAME or defaultRegion
// "eu-de"
func GetRegion(authOpts golangsdk.AuthOptions) string {
	n := authOpts.TenantName
	if n == "" {
		n = authOpts.DelegatedProject
	}
	defRegion := defaultRegion
	if len(n) != 0 {
		defRegion = strings.Split(n, "_")[0]
	}
	return getenv("OS_REGION_NAME", defRegion)
}

//getenv returns value from env is present or default value
func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
