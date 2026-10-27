package provider

import (
	"context"
	"github.com/M1778/infinity-iac/internal/domain"
	"path/filepath"
	"testing"
)

func TestFileProviderIdempotentCreateAndLifecycle(t *testing.T) {
	p := NewFileProvider(filepath.Join(t.TempDir(), "state.json"))
	s := domain.ServerSpec{Name: "demo", Provider: "local", OS: "ubuntu-latest", CPU: 1, RAMGB: 1, StorageGB: 20}
	a, err := p.CreateInstance(context.Background(), CreateRequest{Spec: s, IdempotencyKey: "op-1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.CreateInstance(context.Background(), CreateRequest{Spec: s, IdempotencyKey: "op-1"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Fatalf("not idempotent: %s != %s", a.ID, b.ID)
	}
	if _, err := p.StopInstance(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.RestartInstance(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	if err := p.DeleteInstance(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	got, err := p.ListInstances(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("deleted instance listed: %+v", got)
	}
}
