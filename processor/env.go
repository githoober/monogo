package processor

import (
	"os"

	"github.com/githoober/monogo"
)

// Env adds specified environment variables to Extra["env"].
// Variables that are not set in the environment are omitted.
func Env(keys ...string) monogo.ProcessorFunc {
	envMap := make(map[string]interface{}, len(keys))
	for _, k := range keys {
		if val, exists := os.LookupEnv(k); exists {
			envMap[k] = val
		}
	}

	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		if existing, ok := r.Extra["env"].(map[string]interface{}); ok {
			for k, v := range envMap {
				existing[k] = v
			}
		} else {
			cp := make(map[string]interface{}, len(envMap))
			for k, v := range envMap {
				cp[k] = v
			}
			r.Extra["env"] = cp
		}
		return r
	}
}

// EnvMap maps environment variable names directly to top-level attributes in Extra.
// The map key is the environment variable name (e.g. "APP_ENV");
// the map value is the target key name in Extra (e.g. "environment").
func EnvMap(mapping map[string]string) monogo.ProcessorFunc {
	mapped := make(map[string]interface{}, len(mapping))
	for envVar, extraKey := range mapping {
		if val, exists := os.LookupEnv(envVar); exists {
			mapped[extraKey] = val
		}
	}

	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		for k, v := range mapped {
			r.Extra[k] = v
		}
		return r
	}
}
