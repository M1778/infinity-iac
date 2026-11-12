package infinityiac_test

import (
	"strings"
	"testing"
	"time"

	iac "github.com/M1778/infinity-iac"
)

func TestExternalCatalogAndRenderingAPI(t *testing.T) {
	catalog, err := iac.BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.List()) < 100 {
		t.Fatal("public catalog is missing built-in recipes")
	}
	item, ok := catalog.Get("nginx")
	if !ok {
		t.Fatal("nginx not found")
	}
	data, err := iac.RenderCloudInit(item, iac.RenderOptions{Port: 18080})
	if err != nil || !strings.Contains(data, "127.0.0.1:18080:80") {
		t.Fatalf("render result: %v", err)
	}
}

func TestExternalPlanAPIUsesStableHash(t *testing.T) {
	manifest := iac.Manifest{APIVersion: "v1", Servers: []iac.ServerSpec{{Name: "example", Provider: "oracle", Access: "automation", Region: "us-phoenix-1", UserData: "#!/bin/bash\nprintf ready\n"}}}
	first, err := iac.NewPlan(manifest, nil, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	second, err := iac.NewPlan(manifest, nil, time.Unix(2, 0))
	if err != nil || first.Hash != second.Hash || !first.ValidHash() {
		t.Fatal("review hash must not depend on execution time")
	}
	manifest.Servers[0].UserData = "different bootstrap"
	changed, err := iac.NewPlan(manifest, nil, time.Unix(1, 0))
	if err != nil || changed.Hash == first.Hash {
		t.Fatal("bootstrap changes must invalidate a reviewed plan")
	}
}

func TestExternalProvidersFailWithoutRealConfiguration(t *testing.T) {
	if _, err := iac.NewOCIProvider(iac.AccessProfile{Provider: "oracle"}); err == nil {
		t.Fatal("compute provider accepted missing configuration")
	}
	if _, err := iac.NewOCINetworkProvider(iac.AccessProfile{Provider: "oracle"}); err == nil {
		t.Fatal("network provider accepted missing configuration")
	}
}
