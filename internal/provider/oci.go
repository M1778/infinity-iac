package provider

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

var ErrOCIConfiguration = errors.New("OCI configuration is incomplete")

type OCIProvider struct {
	profile      domain.AccessProfile
	compute      core.ComputeClient
	network      core.VirtualNetworkClient
	identity     identity.IdentityClient
	block        core.BlockstorageClient
	tenancyID    string
	pollInterval time.Duration
}

func (p *OCIProvider) Profile() domain.AccessProfile { return p.profile }

func NewOCIProvider(profile domain.AccessProfile) (*OCIProvider, error) {
	if profile.SubnetID == "" || profile.AvailabilityDomain == "" || profile.ImageID == "" || profile.Shape == "" {
		return nil, ErrOCIConfiguration
	}
	config, tenancyID, err := loadOCIConfiguration(profile)
	if err != nil {
		return nil, err
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
	blockClient, err := core.NewBlockstorageClientWithConfigurationProvider(config)
	if err != nil {
		return nil, fmt.Errorf("create OCI block-storage client: %w", err)
	}
	blockClient.SetRegion(profile.Region)
	configureOCIClient(&compute.BaseClient)
	configureOCIClient(&network.BaseClient)
	configureOCIClient(&identityClient.BaseClient)
	configureOCIClient(&blockClient.BaseClient)
	return &OCIProvider{profile: profile, compute: compute, network: network, identity: identityClient, block: blockClient, tenancyID: tenancyID}, nil
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
	if err := validateOCICompartment(ctx, p.identity, p.profile.CompartmentID); err != nil {
		return err
	}

	subnetResp, err := p.network.GetSubnet(ociRequestContext(ctx), core.GetSubnetRequest{SubnetId: common.String(p.profile.SubnetID)})
	if err != nil {
		return fmt.Errorf("inspect OCI subnet: %w", err)
	}
	if subnetResp.Id == nil || *subnetResp.Id != p.profile.SubnetID {
		return errors.New("OCI subnet response did not match the configured subnet")
	}
	if value(subnetResp.CompartmentId) != p.profile.CompartmentID {
		return errors.New("OCI subnet does not belong to the configured compartment")
	}
	if subnetResp.FreeformTags[managedTag] == "true" && subnetResp.FreeformTags[accessTag] != p.profile.Name {
		return ErrOCIOwnership
	}
	if subnetResp.ProhibitPublicIpOnVnic != nil && *subnetResp.ProhibitPublicIpOnVnic {
		return errors.New("OCI subnet does not permit the requested public IP")
	}
	if subnetResp.LifecycleState != core.SubnetLifecycleStateAvailable {
		return fmt.Errorf("OCI subnet is not available: %s", subnetResp.LifecycleState)
	}

	imageResp, err := p.compute.GetImage(ociRequestContext(ctx), core.GetImageRequest{ImageId: common.String(p.profile.ImageID)})
	if err != nil {
		return fmt.Errorf("inspect OCI image: %w", err)
	}
	if imageResp.Id == nil || *imageResp.Id != p.profile.ImageID {
		return errors.New("OCI image response did not match the configured image")
	}
	if imageResp.LifecycleState != core.ImageLifecycleStateAvailable {
		return fmt.Errorf("OCI image is not available: %s", imageResp.LifecycleState)
	}
	if _, err := p.compute.GetImageShapeCompatibilityEntry(ociRequestContext(ctx), core.GetImageShapeCompatibilityEntryRequest{ImageId: common.String(p.profile.ImageID), ShapeName: common.String(p.profile.Shape)}); err != nil {
		return fmt.Errorf("validate OCI image and shape compatibility: %w", err)
	}

	availabilityResp, err := p.identity.ListAvailabilityDomains(ociRequestContext(ctx), identity.ListAvailabilityDomainsRequest{CompartmentId: common.String(p.tenancyID)})
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
	_, err := p.compute.ListInstances(ociRequestContext(ctx), core.ListInstancesRequest{CompartmentId: common.String(p.profile.CompartmentID), Limit: common.Int(1)})
	if err != nil {
		return fmt.Errorf("list OCI instances: %w", err)
	}
	return nil
}

func (p *OCIProvider) ListInstances(ctx context.Context, filter Filter) ([]domain.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if filter.Region != "" && filter.Region != p.profile.Region || filter.Access != "" && filter.Access != p.profile.Name || filter.Provider != "" && filter.Provider != "oracle" && filter.Provider != "oci" {
		return []domain.Instance{}, nil
	}
	result := make([]domain.Instance, 0)
	var page *string
	for {
		req := core.ListInstancesRequest{CompartmentId: common.String(p.profile.CompartmentID), Page: page}
		resp, err := p.compute.ListInstances(ociRequestContext(ctx), req)
		if err != nil {
			return nil, fmt.Errorf("list OCI instances: %w", err)
		}
		for _, item := range resp.Items {
			if value(item.Id) == "" || value(item.CompartmentId) != p.profile.CompartmentID || item.FreeformTags[managedTag] != "true" || item.LifecycleState == core.InstanceLifecycleStateTerminated {
				continue
			}
			if access := item.FreeformTags[accessTag]; access != "" && access != p.profile.Name {
				continue
			}
			instance, err := p.describeInstance(ctx, item)
			if err != nil {
				return nil, err
			}
			result = append(result, instance)
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
	token, err := operationToken(req.IdempotencyKey)
	if err != nil {
		return domain.Instance{}, err
	}
	if existing, err := p.existingLaunch(ctx, req.Spec.Name, token); err != nil {
		return domain.Instance{}, err
	} else if existing != nil {
		return *existing, nil
	}
	if err := p.validateLaunchSubnet(ctx); err != nil {
		return domain.Instance{}, err
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
	details := core.LaunchInstanceDetails{
		AvailabilityDomain: common.String(p.profile.AvailabilityDomain),
		CompartmentId:      common.String(p.profile.CompartmentID),
		DisplayName:        common.String(req.Spec.Name),
		Shape:              common.String(p.profile.Shape),
		SourceDetails:      source,
		CreateVnicDetails:  &core.CreateVnicDetails{SubnetId: common.String(p.profile.SubnetID), AssignPublicIp: common.Bool(true)},
		Metadata:           metadata,
		FreeformTags: map[string]string{
			managedTag: "true", accessTag: p.profile.Name, operationTag: token,
			"infinity-iac-name": req.Spec.Name, "infinity-iac-subnet": p.profile.SubnetID,
			"infinity-iac-os": req.Spec.OS, "infinity-iac-port": strconv.Itoa(req.Spec.Port),
			"infinity-iac-storage": strconv.Itoa(effectiveStorage),
		},
	}
	effectiveCPU := req.Spec.CPU
	if strings.Contains(strings.ToLower(p.profile.Shape), "flex") {
		ocpus := float32(req.Spec.CPU) / float32(ociVCPUPerOCPU(p.profile.Shape))
		if ocpus < 1 {
			ocpus = 1
		}
		effectiveCPU = int(ocpus * float32(ociVCPUPerOCPU(p.profile.Shape)))
		memory := float32(req.Spec.RAMGB)
		details.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{Ocpus: &ocpus, MemoryInGBs: &memory}
	}
	details.FreeformTags["infinity-iac-cpu"] = strconv.Itoa(effectiveCPU)
	resp, err := p.compute.LaunchInstance(ociRequestContext(ctx), core.LaunchInstanceRequest{LaunchInstanceDetails: details, OpcRetryToken: common.String(token)})
	if err != nil {
		return domain.Instance{}, fmt.Errorf("launch OCI instance: %w", err)
	}
	if value(resp.Id) == "" {
		return domain.Instance{}, errors.New("OCI launch returned no instance ID")
	}
	instance := p.instanceWithSpecs(*resp.Id, req.Spec.Name, string(resp.LifecycleState), p.profile.Shape, p.profile.Region, "", effectiveCPU, req.Spec.RAMGB, effectiveStorage)
	instance.OS = req.Spec.OS
	instance.Port = req.Spec.Port
	instance.Metadata[operationTag] = token
	instance.Metadata[accessTag] = p.profile.Name
	instance.Metadata["infinity-iac-subnet"] = p.profile.SubnetID
	return instance, nil
}

func (p *OCIProvider) StopInstance(ctx context.Context, id string) (domain.Instance, error) {
	return p.powerAction(ctx, id, core.InstanceActionActionStop)
}

func (p *OCIProvider) StartInstance(ctx context.Context, id string) (domain.Instance, error) {
	return p.powerAction(ctx, id, core.InstanceActionActionStart)
}

func (p *OCIProvider) RestartInstance(ctx context.Context, id string) (domain.Instance, error) {
	resp, err := p.getOwnedInstance(ctx, id)
	if err != nil {
		return domain.Instance{}, err
	}
	action := core.InstanceActionActionSoftreset
	if resp.LifecycleState == core.InstanceLifecycleStateStopped {
		action = core.InstanceActionActionStart
	}
	return p.performPowerAction(ctx, resp, action)
}

func (p *OCIProvider) powerAction(ctx context.Context, id string, action core.InstanceActionActionEnum) (domain.Instance, error) {
	resp, err := p.getOwnedInstance(ctx, id)
	if err != nil {
		return domain.Instance{}, err
	}
	return p.performPowerAction(ctx, resp, action)
}

func (p *OCIProvider) performPowerAction(ctx context.Context, current core.GetInstanceResponse, action core.InstanceActionActionEnum) (domain.Instance, error) {
	if action == core.InstanceActionActionStart && current.LifecycleState == core.InstanceLifecycleStateRunning || action == core.InstanceActionActionStop && current.LifecycleState == core.InstanceLifecycleStateStopped {
		return p.describeInstance(ctx, current.Instance)
	}
	if action == core.InstanceActionActionStart && current.LifecycleState != core.InstanceLifecycleStateStopped || action != core.InstanceActionActionStart && current.LifecycleState != core.InstanceLifecycleStateRunning {
		return domain.Instance{}, fmt.Errorf("cannot perform OCI %s while instance is %s", action, current.LifecycleState)
	}
	match, err := etag(current.Etag)
	if err != nil {
		return domain.Instance{}, err
	}
	resp, err := p.compute.InstanceAction(ociRequestContext(ctx), core.InstanceActionRequest{InstanceId: current.Id, Action: action, IfMatch: match})
	if err != nil {
		return domain.Instance{}, fmt.Errorf("OCI instance action %s: %w", action, err)
	}
	return p.describeInstance(ctx, resp.Instance)
}

func (p *OCIProvider) DeleteInstance(ctx context.Context, id string) error {
	resp, err := p.getOwnedInstance(ctx, id)
	if err != nil {
		return err
	}
	return p.terminateOwnedInstance(ctx, resp)
}

func (p *OCIProvider) terminateOwnedInstance(ctx context.Context, resp core.GetInstanceResponse) error {
	if resp.LifecycleState == core.InstanceLifecycleStateTerminated || resp.LifecycleState == core.InstanceLifecycleStateTerminating {
		return nil
	}
	match, err := etag(resp.Etag)
	if err != nil {
		return err
	}
	if _, err := p.compute.TerminateInstance(ociRequestContext(ctx), core.TerminateInstanceRequest{InstanceId: resp.Id, IfMatch: match, PreserveBootVolume: common.Bool(false)}); err != nil {
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
