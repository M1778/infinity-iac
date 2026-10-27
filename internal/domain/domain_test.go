package domain

import (
	"testing"
	"time"
)

func TestResolveDefaultsAndValidate(t *testing.T) {
	s := (ServerSpec{Name: "demo"}).ResolveDefaults("oracle", "default", "us-phoenix-1")
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.OS != "ubuntu-latest" || s.CPU != 1 || s.RAMGB != 1 || s.StorageGB != 20 || s.NetworkingScript != "iauto" {
		t.Fatalf("unexpected defaults: %+v", s)
	}
}
func TestPlanOnlyCreatesMissingResources(t *testing.T) {
	p, err := NewPlan(Manifest{Servers: []ServerSpec{{Name: "new", Provider: "local", OS: "ubuntu-latest", CPU: 1, RAMGB: 1, StorageGB: 20}}}, []Instance{{Name: "old", Status: StatusRunning}}, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Actions) != 1 || p.Actions[0].Resource != "new" {
		t.Fatalf("unexpected plan: %+v", p)
	}
}
