package provider

import (
	"context"
	"github.com/M1778/infinity-iac/internal/domain"
)

type CreateRequest struct {
	Spec           domain.ServerSpec
	IdempotencyKey string
	// UserData is cloud-init content. Providers that support instance
	// bootstrap must encode it according to their API contract.
	UserData string
}

type Filter struct{ Provider, Access, Region string }

type Provider interface {
	Name() string
	ListInstances(context.Context, Filter) ([]domain.Instance, error)
	CreateInstance(context.Context, CreateRequest) (domain.Instance, error)
	StopInstance(context.Context, string) (domain.Instance, error)
	RestartInstance(context.Context, string) (domain.Instance, error)
	DeleteInstance(context.Context, string) error
}
