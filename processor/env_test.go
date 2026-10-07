package processor_test

import (
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/processor"
)

func TestEnvProcessors(t *testing.T) {
	t.Setenv("MONOGO_CORE_CLUSTER", "k8s-prod-core")
	t.Setenv("MONOGO_CORE_REGION", "us-east-1")

	// Test Env with multiple keys and missing key
	envProc := processor.Env("MONOGO_CORE_CLUSTER", "MONOGO_CORE_REGION", "NON_EXISTENT_VAR")
	rec := envProc.Process(monogo.Record{Message: "env test"})

	envMap, ok := rec.Extra["env"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Extra['env'] to be map, got: %v", rec.Extra["env"])
	}
	if envMap["MONOGO_CORE_CLUSTER"] != "k8s-prod-core" {
		t.Errorf("expected cluster 'k8s-prod-core', got: %v", envMap["MONOGO_CORE_CLUSTER"])
	}
	if envMap["MONOGO_CORE_REGION"] != "us-east-1" {
		t.Errorf("expected region 'us-east-1', got: %v", envMap["MONOGO_CORE_REGION"])
	}
	if _, exists := envMap["NON_EXISTENT_VAR"]; exists {
		t.Errorf("non-existent var should not be present in env map")
	}

	// Test existing Extra["env"] merging
	recExisting := monogo.Record{
		Message: "existing env test",
		Extra: map[string]interface{}{
			"env": map[string]interface{}{
				"EXISTING_KEY": "existing_val",
			},
		},
	}
	recExisting = envProc.Process(recExisting)
	mergedEnv := recExisting.Extra["env"].(map[string]interface{})
	if mergedEnv["EXISTING_KEY"] != "existing_val" || mergedEnv["MONOGO_CORE_CLUSTER"] != "k8s-prod-core" {
		t.Errorf("expected merged keys, got: %v", mergedEnv)
	}

	// Test EnvMap top-level key mapping
	envMapProc := processor.EnvMap(map[string]string{
		"MONOGO_CORE_CLUSTER": "cluster",
		"MONOGO_CORE_REGION":  "region",
	})
	rec2 := envMapProc.Process(monogo.Record{Message: "env map test"})
	if rec2.Extra["cluster"] != "k8s-prod-core" {
		t.Errorf("expected extra['cluster']='k8s-prod-core', got: %v", rec2.Extra["cluster"])
	}
	if rec2.Extra["region"] != "us-east-1" {
		t.Errorf("expected extra['region']='us-east-1', got: %v", rec2.Extra["region"])
	}
}
