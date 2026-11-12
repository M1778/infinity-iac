package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

const (
	managedTag   = "infinity-iac"
	accessTag    = "infinity-iac-access"
	networkTag   = "infinity-iac-network"
	operationTag = "infinity-iac-operation"
)

var ErrOCIOwnership = errors.New("OCI resource is not owned by the selected access profile")

type ociTransportContext struct{}

type ociContextDispatcher struct {
	inner common.HTTPRequestDispatcher
}

func (d ociContextDispatcher) Do(request *http.Request) (*http.Response, error) {
	if ctx, ok := request.Context().Value(ociTransportContext{}).(context.Context); ok {
		request = request.WithContext(ctx)
	}
	return d.inner.Do(request)
}

func configureOCIClient(client *common.BaseClient) {
	noRetry := common.NoRetryPolicy()
	client.Configuration.RetryPolicy = &noRetry
	client.HTTPClient = ociContextDispatcher{inner: client.HTTPClient}
}

// SDK retries read shared response state on cancellation; transport cancellation still uses the caller's context.
func ociRequestContext(ctx context.Context) context.Context {
	return context.WithValue(context.WithoutCancel(ctx), ociTransportContext{}, ctx)
}

func loadOCIConfiguration(profile domain.AccessProfile) (common.ConfigurationProvider, string, error) {
	if profile.Provider != "oracle" && profile.Provider != "oci" {
		return nil, "", fmt.Errorf("unsupported OCI profile provider %q", profile.Provider)
	}
	if strings.TrimSpace(profile.Name) == "" || profile.CredentialSource == "" || profile.Region == "" || profile.CompartmentID == "" {
		return nil, "", ErrOCIConfiguration
	}
	parts := strings.SplitN(profile.CredentialSource, ":", 2)
	if len(parts) != 2 || parts[0] != "oci-profile" || strings.TrimSpace(parts[1]) == "" {
		return nil, "", errors.New("invalid OCI credential source")
	}
	path, err := ociConfigPath(profile.ConfigPath)
	if err != nil {
		return nil, "", err
	}
	config, err := common.ConfigurationProviderFromFileWithProfile(path, parts[1], "")
	if err != nil {
		return nil, "", fmt.Errorf("load OCI profile %q from %s: %w", parts[1], path, err)
	}
	if ok, err := common.IsConfigurationProviderValid(config); !ok {
		if err == nil {
			err = errors.New("OCI configuration provider is invalid")
		}
		return nil, "", fmt.Errorf("load OCI profile %q: %w", parts[1], err)
	}
	region, err := config.Region()
	if err != nil {
		return nil, "", fmt.Errorf("read OCI profile region: %w", err)
	}
	if region != profile.Region {
		return nil, "", fmt.Errorf("OCI profile region %q does not match requested region %q", region, profile.Region)
	}
	tenancyID, err := config.TenancyOCID()
	if err != nil {
		return nil, "", fmt.Errorf("read OCI tenancy OCID: %w", err)
	}
	return config, tenancyID, nil
}

func validateOCICompartment(ctx context.Context, client identity.IdentityClient, compartmentID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resp, err := client.GetCompartment(ociRequestContext(ctx), identity.GetCompartmentRequest{CompartmentId: common.String(compartmentID)})
	if err != nil {
		return fmt.Errorf("inspect OCI compartment: %w", err)
	}
	if value(resp.Id) != compartmentID {
		return errors.New("OCI compartment response did not match the configured compartment")
	}
	if resp.LifecycleState != identity.CompartmentLifecycleStateActive {
		return fmt.Errorf("OCI compartment is not active: %s", resp.LifecycleState)
	}
	return nil
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ownOCIResource(profile domain.AccessProfile, compartmentID *string, tags map[string]string) error {
	if value(compartmentID) != profile.CompartmentID || tags[managedTag] != "true" || tags[accessTag] != profile.Name {
		return ErrOCIOwnership
	}
	return nil
}

func isOCINotFound(err error) bool {
	var serviceErr common.ServiceError
	return errors.As(err, &serviceErr) && serviceErr.GetHTTPStatusCode() == 404
}

func etag(p *string) (*string, error) {
	if value(p) == "" {
		return nil, errors.New("OCI resource response has no ETag; refusing an unguarded mutation")
	}
	return p, nil
}

func pollOCI(ctx context.Context, interval time.Duration, check func() (bool, error)) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		done, err := check()
		if err != nil || done {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func boundedOCIContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 10*time.Minute)
}

func operationToken(token string) (string, error) {
	if token == "" {
		return common.RetryToken(), nil
	}
	if len(token) > 64 || strings.ContainsAny(token, " \t\r\n") {
		return "", errors.New("OCI idempotency key must be at most 64 characters without whitespace")
	}
	return token, nil
}

func childOperationToken(token, kind string) string {
	hash := sha256.Sum256([]byte(token + ":" + kind))
	return hex.EncodeToString(hash[:])
}
