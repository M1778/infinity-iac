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
	GetInstance(context.Context, string) (domain.Instance, error)
	StartInstance(context.Context, string) (domain.Instance, error)
	StopInstance(context.Context, string) (domain.Instance, error)
	RestartInstance(context.Context, string) (domain.Instance, error)
	WaitInstance(context.Context, string, string) (domain.Instance, error)
	DeleteInstance(context.Context, string) error
	DeleteInstanceAndWait(context.Context, string) error
}

type NetworkCreateRequest struct {
	Spec           domain.NetworkSpec
	IdempotencyKey string
}

type NetworkProvider interface {
	CreateNetwork(context.Context, NetworkCreateRequest) (domain.Network, error)
	ListNetworks(context.Context) ([]domain.Network, error)
	GetNetwork(context.Context, string) (domain.Network, error)
	UpdateNetwork(context.Context, string, string) (domain.Network, error)
	WaitNetwork(context.Context, string, string) (domain.Network, error)
	DeleteNetwork(context.Context, string) error
}
