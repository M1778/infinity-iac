package provider

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

var ErrOCIConfiguration = errors.New("OCI configuration is incomplete")

type OCIProvider struct {
	profile   domain.AccessProfile
	compute   core.ComputeClient
	network   core.VirtualNetworkClient
	identity  identity.IdentityClient
	tenancyID string
}

func (p *OCIProvider) Profile() domain.AccessProfile { return p.profile }

func NewOCIProvider(profile domain.AccessProfile) (*OCIProvider, error) {
	if profile.Provider != "oracle" && profile.Provider != "oci" {
		return nil, fmt.Errorf("unsupported OCI profile provider %q", profile.Provider)
	}
	if profile.CredentialSource == "" || profile.Region == "" || profile.CompartmentID == "" || profile.SubnetID == "" || profile.AvailabilityDomain == "" || profile.ImageID == "" || profile.Shape == "" {
		return nil, ErrOCIConfiguration
	}
	parts := strings.SplitN(profile.CredentialSource, ":", 2)
	if len(parts) != 2 || parts[0] != "oci-profile" || strings.TrimSpace(parts[1]) == "" {
		return nil, errors.New("invalid OCI credential source")
	}
	configPath, err := ociConfigPath(profile.ConfigPath)
	if err != nil {
		return nil, err
	}
	config, err := common.ConfigurationProviderFromFileWithProfile(configPath, parts[1], "")
	if err != nil {
		return nil, fmt.Errorf("load OCI profile %q from %s: %w", parts[1], configPath, err)
	}
	if ok, err := common.IsConfigurationProviderValid(config); !ok {
		if err == nil {
			err = errors.New("OCI configuration provider is invalid")
		}
		return nil, fmt.Errorf("load OCI profile %q: %w", parts[1], err)
	}
	configuredRegion, err := config.Region()
	if err != nil {
		return nil, fmt.Errorf("read OCI profile region: %w", err)
	}
	if configuredRegion != profile.Region {
		return nil, fmt.Errorf("OCI profile region %q does not match requested region %q", configuredRegion, profile.Region)
	}
	tenancyID, err := config.TenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("read OCI tenancy OCID: %w", err)
	}
	compute, err := core.NewComputeClientWithConfigurationProvider(config)
	if err != nil {
		return nil, fmt.Errorf("create OCI compute client: %w", err)
	}
	compute.SetRegion(profile.Region)
	network, err := core.NewVirtualNetworkClientWithConfigurationProvider(config)
	if err != nil {
		return nil, fmt.Errorf("create OCI network client: %w", err)
	}
	network.SetRegion(profile.Region)
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(config)
	if err != nil {
		return nil, fmt.Errorf("create OCI identity client: %w", err)
	}
	identityClient.SetRegion(profile.Region)
	return &OCIProvider{profile: profile, compute: compute, network: network, identity: identityClient, tenancyID: tenancyID}, nil
}

func ociConfigPath(configPath string) (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	if configPath := os.Getenv("OCI_CONFIG_FILE"); configPath != "" {
		return configPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve OCI config path: %w", err)
	}
	return filepath.Join(home, ".oci", "config"), nil
}

func (p *OCIProvider) Name() string { return "oracle" }

// ValidateAccess performs only read operations. It verifies that the selected
// OCI profile can authenticate and inspect every resource required by a future
// launch, then checks the compartment instance-list permission. No profile is
// saved and no cloud resource is created by this method.
func (p *OCIProvider) ValidateAccess(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	compartmentResp, err := p.identity.GetCompartment(ctx, identity.GetCompartmentRequest{CompartmentId: common.String(p.profile.CompartmentID)})
	if err != nil {
		return fmt.Errorf("inspect OCI compartment: %w", err)
	}
	if compartmentResp.Id == nil || *compartmentResp.Id != p.profile.CompartmentID {
		return errors.New("OCI compartment response did not match the configured compartment")
	}
	if compartmentResp.LifecycleState != identity.CompartmentLifecycleStateActive {
		return fmt.Errorf("OCI compartment is not active: %s", compartmentResp.LifecycleState)
	}

	subnetResp, err := p.network.GetSubnet(ctx, core.GetSubnetRequest{SubnetId: common.String(p.profile.SubnetID)})
	if err != nil {
		return fmt.Errorf("inspect OCI subnet: %w", err)
	}
	if subnetResp.Id == nil || *subnetResp.Id != p.profile.SubnetID {
		return errors.New("OCI subnet response did not match the configured subnet")
	}
	if subnetResp.LifecycleState != core.SubnetLifecycleStateAvailable {
		return fmt.Errorf("OCI subnet is not available: %s", subnetResp.LifecycleState)
	}

	imageResp, err := p.compute.GetImage(ctx, core.GetImageRequest{ImageId: common.String(p.profile.ImageID)})
	if err != nil {
		return fmt.Errorf("inspect OCI image: %w", err)
	}
	if imageResp.Id == nil || *imageResp.Id != p.profile.ImageID {
		return errors.New("OCI image response did not match the configured image")
	}
	if imageResp.LifecycleState != core.ImageLifecycleStateAvailable {
		return fmt.Errorf("OCI image is not available: %s", imageResp.LifecycleState)
	}
	if _, err := p.compute.GetImageShapeCompatibilityEntry(ctx, core.GetImageShapeCompatibilityEntryRequest{ImageId: common.String(p.profile.ImageID), ShapeName: common.String(p.profile.Shape)}); err != nil {
		return fmt.Errorf("validate OCI image and shape compatibility: %w", err)
	}

	availabilityResp, err := p.identity.ListAvailabilityDomains(ctx, identity.ListAvailabilityDomainsRequest{CompartmentId: common.String(p.tenancyID)})
	if err != nil {
		return fmt.Errorf("list OCI availability domains: %w", err)
	}
	foundAvailabilityDomain := false
	for _, item := range availabilityResp.Items {
		if item.Name != nil && *item.Name == p.profile.AvailabilityDomain {
			foundAvailabilityDomain = true
			break
		}
	}
	if !foundAvailabilityDomain {
		return fmt.Errorf("OCI availability domain %q was not found in the configured tenancy", p.profile.AvailabilityDomain)
	}
	if err := p.validateInstanceList(ctx); err != nil {
		return err
	}
	return nil
}

func (p *OCIProvider) validateInstanceList(ctx context.Context) error {
	_, err := p.compute.ListInstances(ctx, core.ListInstancesRequest{CompartmentId: common.String(p.profile.CompartmentID), Limit: common.Int(1)})
	if err != nil {
		return fmt.Errorf("list OCI instances: %w", err)
	}
	return nil
}

func (p *OCIProvider) ListInstances(ctx context.Context, filter Filter) ([]domain.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if filter.Region != "" && filter.Region != p.profile.Region {
		return []domain.Instance{}, nil
	}
	result := make([]domain.Instance, 0)
	var page *string
	for {
		req := core.ListInstancesRequest{CompartmentId: common.String(p.profile.CompartmentID), Page: page}
		resp, err := p.compute.ListInstances(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("list OCI instances: %w", err)
		}
		for _, item := range resp.Items {
			if item.Id == nil || filter.Access != "" && filter.Access != p.profile.Name {
				continue
			}
			if item.FreeformTags["infinity-iac"] != "true" {
				continue
			}
			if string(item.LifecycleState) == "TERMINATED" {
				continue
			}
			name := ""
			if item.DisplayName != nil {
				name = *item.DisplayName
			}
			shape := ""
			if item.Shape != nil {
				shape = *item.Shape
			}
			cpu, ram := 0, 0
			if item.ShapeConfig != nil {
				if item.ShapeConfig.Vcpus != nil {
					cpu = *item.ShapeConfig.Vcpus
				} else if item.ShapeConfig.Ocpus != nil {
					cpu = int(*item.ShapeConfig.Ocpus)
				}
				if item.ShapeConfig.MemoryInGBs != nil {
					ram = int(*item.ShapeConfig.MemoryInGBs)
				}
			}
			region := p.profile.Region
			if item.Region != nil {
				region = *item.Region
			}
			if filter.Provider != "" && filter.Provider != "oracle" && filter.Provider != "oci" {
				continue
			}
			publicIP := ""
			attachments, attachErr := p.compute.ListVnicAttachments(ctx, core.ListVnicAttachmentsRequest{CompartmentId: common.String(p.profile.CompartmentID), InstanceId: item.Id})
			if attachErr != nil {
				return nil, fmt.Errorf("list OCI VNIC attachments for %s: %w", *item.Id, attachErr)
			}
			if len(attachments.Items) > 0 && attachments.Items[0].VnicId != nil {
				vnic, vnicErr := p.network.GetVnic(ctx, core.GetVnicRequest{VnicId: attachments.Items[0].VnicId})
				if vnicErr != nil {
					return nil, fmt.Errorf("get OCI VNIC for %s: %w", *item.Id, vnicErr)
				}
				if vnic.PublicIp != nil {
					publicIP = *vnic.PublicIp
				}
			}
			created := time.Time{}
			if item.TimeCreated != nil {
				created = item.TimeCreated.Time
			}
			result = append(result, p.instanceWithSpecs(*item.Id, name, string(item.LifecycleState), shape, region, publicIP, cpu, ram, 0, created))
		}
		page = resp.OpcNextPage
		if page == nil || *page == "" {
			break
		}
	}
	return result, nil
}

func (p *OCIProvider) CreateInstance(ctx context.Context, req CreateRequest) (domain.Instance, error) {
	if err := ctx.Err(); err != nil {
		return domain.Instance{}, err
	}
	if err := req.Spec.Validate(); err != nil {
		return domain.Instance{}, err
	}
	if req.Spec.Provider != "oracle" && req.Spec.Provider != "oci" {
		return domain.Instance{}, fmt.Errorf("OCI provider cannot create a %q resource", req.Spec.Provider)
	}
	if req.Spec.Access != "" && req.Spec.Access != p.profile.Name {
		return domain.Instance{}, fmt.Errorf("access profile %q does not match configured profile %q", req.Spec.Access, p.profile.Name)
	}
	if req.Spec.Region != "" && req.Spec.Region != p.profile.Region {
		return domain.Instance{}, fmt.Errorf("region %q does not match configured region %q", req.Spec.Region, p.profile.Region)
	}
	if strings.TrimSpace(req.Spec.NetworkingScript) != "" {
		return domain.Instance{}, errors.New("OCI networking scripts are not supported in this release")
	}
	metadata := map[string]string{}
	if p.profile.SSHPublicKey != "" {
		metadata["ssh_authorized_keys"] = p.profile.SSHPublicKey
	}
	if req.UserData != "" {
		metadata["user_data"] = base64.StdEncoding.EncodeToString([]byte(req.UserData))
		if len(metadata["user_data"])+len(metadata["ssh_authorized_keys"]) > 30000 {
			return domain.Instance{}, errors.New("OCI cloud-init user data exceeds the metadata size limit")
		}
	}
	source := &core.InstanceSourceViaImageDetails{ImageId: common.String(p.profile.ImageID)}
	effectiveStorage := req.Spec.StorageGB
	if effectiveStorage < 50 {
		effectiveStorage = 50
	}
	source.BootVolumeSizeInGBs = common.Int64(int64(effectiveStorage))
	details := core.LaunchInstanceDetails{AvailabilityDomain: common.String(p.profile.AvailabilityDomain), CompartmentId: common.String(p.profile.CompartmentID), DisplayName: common.String(req.Spec.Name), Shape: common.String(p.profile.Shape), SourceDetails: source, CreateVnicDetails: &core.CreateVnicDetails{SubnetId: common.String(p.profile.SubnetID), AssignPublicIp: common.Bool(true)}, Metadata: metadata, FreeformTags: map[string]string{"infinity-iac": "true", "infinity-iac-name": req.Spec.Name}}
	if strings.Contains(strings.ToLower(p.profile.Shape), "flex") {
		ocpus := float32(req.Spec.CPU)
		memory := float32(req.Spec.RAMGB)
		details.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{Ocpus: &ocpus, MemoryInGBs: &memory}
	}
	resp, err := p.compute.LaunchInstance(ctx, core.LaunchInstanceRequest{LaunchInstanceDetails: details, OpcRetryToken: common.String(req.IdempotencyKey)})
	if err != nil {
		return domain.Instance{}, fmt.Errorf("launch OCI instance: %w", err)
	}
	if resp.Id == nil {
		return domain.Instance{}, errors.New("OCI launch returned no instance ID")
	}
	instance := p.instanceWithSpecs(*resp.Id, req.Spec.Name, string(resp.LifecycleState), p.profile.Shape, p.profile.Region, "", req.Spec.CPU, req.Spec.RAMGB, effectiveStorage)
	instance.OS = req.Spec.OS
	instance.Port = req.Spec.Port
	return instance, nil
}

func (p *OCIProvider) StopInstance(ctx context.Context, id string) (domain.Instance, error) {
	return p.action(ctx, id, core.InstanceActionActionStop)
}
func (p *OCIProvider) RestartInstance(ctx context.Context, id string) (domain.Instance, error) {
	return p.action(ctx, id, core.InstanceActionActionSoftreset)
}
func (p *OCIProvider) action(ctx context.Context, id string, action core.InstanceActionActionEnum) (domain.Instance, error) {
	if err := ctx.Err(); err != nil {
		return domain.Instance{}, err
	}
	resp, err := p.compute.InstanceAction(ctx, core.InstanceActionRequest{InstanceId: common.String(id), Action: action})
	if err != nil {
		return domain.Instance{}, fmt.Errorf("OCI instance action %s: %w", action, err)
	}
	name := ""
	if resp.DisplayName != nil {
		name = *resp.DisplayName
	}
	shape := ""
	if resp.Shape != nil {
		shape = *resp.Shape
	}
	cpu, ram := 0, 0
	if resp.ShapeConfig != nil {
		if resp.ShapeConfig.Vcpus != nil {
			cpu = *resp.ShapeConfig.Vcpus
		} else if resp.ShapeConfig.Ocpus != nil {
			cpu = int(*resp.ShapeConfig.Ocpus)
		}
		if resp.ShapeConfig.MemoryInGBs != nil {
			ram = int(*resp.ShapeConfig.MemoryInGBs)
		}
	}
	region := p.profile.Region
	if resp.Region != nil {
		region = *resp.Region
	}
	return p.instanceWithSpecs(id, name, string(resp.LifecycleState), shape, region, "", cpu, ram, 0), nil
}

func (p *OCIProvider) DeleteInstance(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := p.compute.TerminateInstance(ctx, core.TerminateInstanceRequest{InstanceId: common.String(id), PreserveBootVolume: common.Bool(false)}); err != nil {
		return fmt.Errorf("terminate OCI instance: %w", err)
	}
	return nil
}

func (p *OCIProvider) instance(id, name, state, shape, region, publicIP string, created ...time.Time) domain.Instance {
	return p.instanceWithSpecs(id, name, state, shape, region, publicIP, 0, 0, 0, created...)
}

func (p *OCIProvider) instanceWithSpecs(id, name, state, shape, region, publicIP string, cpu, ram, storage int, created ...time.Time) domain.Instance {
	status := domain.StatusProvisioning
	switch {
	case strings.EqualFold(state, "RUNNING"):
		status = domain.StatusRunning
	case strings.EqualFold(state, "STOPPED"):
		status = domain.StatusStopped
	case strings.EqualFold(state, "TERMINATED"):
		status = domain.StatusDeleted
	}
	now := time.Now().UTC()
	if len(created) > 0 && !created[0].IsZero() {
		now = created[0].UTC()
	}
	return domain.Instance{ID: id, Name: name, Provider: "oracle", Access: p.profile.Name, Region: region, OS: p.profile.ImageID, Status: status, PublicIP: publicIP, CPU: cpu, RAMGB: ram, StorageGB: storage, Owner: "infinity-iac", Metadata: map[string]string{"shape": shape}, CreatedAt: now, UpdatedAt: now}
}
