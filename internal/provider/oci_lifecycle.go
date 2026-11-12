package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func (p *OCIProvider) Architecture() string {
	if strings.HasPrefix(p.profile.Shape, "VM.Standard.A1.") || strings.HasPrefix(p.profile.Shape, "BM.Standard.A1.") {
		return "arm64"
	}
	if strings.Contains(p.profile.Shape, ".Standard.A") {
		return ""
	}
	if strings.Contains(p.profile.Shape, ".Standard.") || strings.Contains(p.profile.Shape, ".Standard1.") || strings.Contains(p.profile.Shape, ".Standard2.") || strings.Contains(p.profile.Shape, ".Standard3.") {
		return "amd64"
	}
	return ""
}

func ociVCPUPerOCPU(shape string) int {
	if strings.HasPrefix(shape, "VM.Standard.A1.") || strings.HasPrefix(shape, "BM.Standard.A1.") {
		return 1
	}
	return 2
}

func (p *OCIProvider) OperatingSystem(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	resp, err := p.compute.GetImage(ociRequestContext(ctx), core.GetImageRequest{ImageId: common.String(p.profile.ImageID)})
	if err != nil {
		return "", fmt.Errorf("inspect OCI image operating system: %w", err)
	}
	if value(resp.Id) != p.profile.ImageID || resp.LifecycleState != core.ImageLifecycleStateAvailable {
		return "", errors.New("OCI image is not the configured available image")
	}
	os := strings.ToLower(strings.TrimSpace(value(resp.OperatingSystem)))
	version := strings.TrimSpace(value(resp.OperatingSystemVersion))
	if os == "canonical ubuntu" {
		if fields := strings.Fields(version); len(fields) > 0 {
			return "ubuntu-" + fields[0], nil
		}
	}
	if os == "" || version == "" {
		return "", errors.New("OCI image has no operating system label")
	}
	return strings.ReplaceAll(os, " ", "-") + "-" + version, nil
}

func (p *OCIProvider) validateLaunchSubnet(ctx context.Context) error {
	resp, err := p.network.GetSubnet(ociRequestContext(ctx), core.GetSubnetRequest{SubnetId: common.String(p.profile.SubnetID)})
	if err != nil {
		return fmt.Errorf("inspect OCI launch subnet: %w", err)
	}
	if value(resp.Id) != p.profile.SubnetID || value(resp.CompartmentId) != p.profile.CompartmentID || resp.LifecycleState != core.SubnetLifecycleStateAvailable {
		return errors.New("OCI launch subnet is not the configured available subnet in the configured compartment")
	}
	if resp.ProhibitPublicIpOnVnic != nil && *resp.ProhibitPublicIpOnVnic {
		return errors.New("OCI launch subnet does not permit a public IP")
	}
	if resp.FreeformTags[managedTag] == "true" && resp.FreeformTags[accessTag] != p.profile.Name {
		return ErrOCIOwnership
	}
	if value(resp.AvailabilityDomain) != "" && value(resp.AvailabilityDomain) != p.profile.AvailabilityDomain {
		return errors.New("OCI subnet is not available in the configured availability domain")
	}
	return nil
}

func (p *OCIProvider) getOwnedInstance(ctx context.Context, id string) (core.GetInstanceResponse, error) {
	if err := ctx.Err(); err != nil {
		return core.GetInstanceResponse{}, err
	}
	if strings.TrimSpace(id) == "" {
		return core.GetInstanceResponse{}, errors.New("instance ID is required")
	}
	resp, err := p.compute.GetInstance(ociRequestContext(ctx), core.GetInstanceRequest{InstanceId: common.String(id)})
	if err != nil {
		return resp, fmt.Errorf("get OCI instance: %w", err)
	}
	if value(resp.Id) != id {
		return resp, errors.New("OCI instance response did not match the requested ID")
	}
	if err := ownOCIResource(p.profile, resp.CompartmentId, resp.FreeformTags); err != nil {
		return resp, err
	}
	return resp, nil
}

func (p *OCIProvider) GetInstance(ctx context.Context, id string) (domain.Instance, error) {
	resp, err := p.getOwnedInstance(ctx, id)
	if err != nil {
		return domain.Instance{}, err
	}
	return p.describeInstance(ctx, resp.Instance)
}

func (p *OCIProvider) existingLaunch(ctx context.Context, name, token string) (*domain.Instance, error) {
	var found *core.Instance
	var page *string
	for {
		resp, err := p.compute.ListInstances(ociRequestContext(ctx), core.ListInstancesRequest{CompartmentId: common.String(p.profile.CompartmentID), DisplayName: common.String(name), Page: page})
		if err != nil {
			return nil, fmt.Errorf("inspect OCI launch identity: %w", err)
		}
		for _, item := range resp.Items {
			if value(item.DisplayName) != name || item.LifecycleState == core.InstanceLifecycleStateTerminated {
				continue
			}
			if ownOCIResource(p.profile, item.CompartmentId, item.FreeformTags) != nil || item.FreeformTags[operationTag] != token || item.FreeformTags["infinity-iac-name"] != name || item.FreeformTags["infinity-iac-subnet"] != p.profile.SubnetID || value(item.Shape) != p.profile.Shape || value(item.AvailabilityDomain) != p.profile.AvailabilityDomain {
				return nil, fmt.Errorf("instance name %q already exists outside this exact launch operation", name)
			}
			if found != nil {
				return nil, fmt.Errorf("multiple OCI instances match launch operation %q", token)
			}
			copy := item
			found = &copy
		}
		page = resp.OpcNextPage
		if value(page) == "" {
			break
		}
	}
	if found == nil {
		return nil, nil
	}
	instance, err := p.describeInstance(ctx, *found)
	return &instance, err
}

func (p *OCIProvider) describeInstance(ctx context.Context, item core.Instance) (domain.Instance, error) {
	cpu, ram := 0, 0
	shape := value(item.Shape)
	if item.ShapeConfig != nil {
		if item.ShapeConfig.Vcpus != nil {
			cpu = *item.ShapeConfig.Vcpus
		} else if item.ShapeConfig.Ocpus != nil {
			cpu = int(*item.ShapeConfig.Ocpus * float32(ociVCPUPerOCPU(shape)))
		}
		if item.ShapeConfig.MemoryInGBs != nil {
			ram = int(*item.ShapeConfig.MemoryInGBs)
		}
	}
	if cpu == 0 {
		cpu, _ = strconv.Atoi(item.FreeformTags["infinity-iac-cpu"])
	}
	storage, _ := strconv.Atoi(item.FreeformTags["infinity-iac-storage"])
	created := time.Time{}
	if item.TimeCreated != nil {
		created = item.TimeCreated.Time
	}
	region := value(item.Region)
	if region == "" {
		region = p.profile.Region
	}
	instance := p.instanceWithSpecs(value(item.Id), value(item.DisplayName), string(item.LifecycleState), shape, region, "", cpu, ram, storage, created)
	if os := item.FreeformTags["infinity-iac-os"]; os != "" {
		instance.OS = os
	}
	instance.Port, _ = strconv.Atoi(item.FreeformTags["infinity-iac-port"])
	for _, key := range []string{operationTag, accessTag, "infinity-iac-subnet"} {
		if tag := item.FreeformTags[key]; tag != "" {
			instance.Metadata[key] = tag
		}
	}
	if item.FreeformTags[accessTag] == "" {
		instance.Access = ""
		instance.Metadata["ownership"] = "legacy-unscoped"
	}
	if item.LifecycleState == core.InstanceLifecycleStateTerminated || item.LifecycleState == core.InstanceLifecycleStateTerminating {
		return instance, nil
	}
	var page *string
	for {
		resp, err := p.compute.ListVnicAttachments(ociRequestContext(ctx), core.ListVnicAttachmentsRequest{CompartmentId: common.String(p.profile.CompartmentID), InstanceId: item.Id, Page: page})
		if err != nil {
			return domain.Instance{}, fmt.Errorf("list OCI VNIC attachments: %w", err)
		}
		for _, attachment := range resp.Items {
			if attachment.LifecycleState != core.VnicAttachmentLifecycleStateAttached || value(attachment.VnicId) == "" || value(attachment.InstanceId) != value(item.Id) {
				continue
			}
			vnic, err := p.network.GetVnic(ociRequestContext(ctx), core.GetVnicRequest{VnicId: attachment.VnicId})
			if err != nil {
				return domain.Instance{}, fmt.Errorf("get OCI VNIC: %w", err)
			}
			if vnic.IsPrimary != nil && *vnic.IsPrimary {
				instance.PublicIP = value(vnic.PublicIp)
				return instance, nil
			}
			if instance.PublicIP == "" {
				instance.PublicIP = value(vnic.PublicIp)
			}
		}
		page = resp.OpcNextPage
		if value(page) == "" {
			break
		}
	}
	return instance, nil
}

func (p *OCIProvider) WaitInstance(ctx context.Context, id, status string) (domain.Instance, error) {
	if status != domain.StatusRunning && status != domain.StatusStopped && status != domain.StatusDeleted {
		return domain.Instance{}, errors.New("wait status must be running, stopped, or deleted")
	}
	ctx, cancel := boundedOCIContext(ctx)
	defer cancel()
	var instance domain.Instance
	var owned bool
	err := pollOCI(ctx, p.pollInterval, func() (bool, error) {
		resp, err := p.getOwnedInstance(ctx, id)
		if err != nil {
			if owned && status == domain.StatusDeleted && isOCINotFound(err) {
				instance.Status = domain.StatusDeleted
				instance.PublicIP = ""
				return true, nil
			}
			return false, err
		}
		owned = true
		instance = p.instanceWithSpecs(id, value(resp.DisplayName), string(resp.LifecycleState), value(resp.Shape), p.profile.Region, "", 0, 0, 0)
		if instance.Status == status {
			instance, err = p.describeInstance(ctx, resp.Instance)
			return true, err
		}
		if resp.LifecycleState == core.InstanceLifecycleStateTerminated || strings.EqualFold(string(resp.LifecycleState), "FAILED") || strings.EqualFold(string(resp.LifecycleState), "FAULTY") {
			return false, fmt.Errorf("instance reached %s while waiting for %s", resp.LifecycleState, status)
		}
		return false, nil
	})
	return instance, err
}

func (p *OCIProvider) DeleteInstanceAndWait(ctx context.Context, id string) error {
	ctx, cancel := boundedOCIContext(ctx)
	defer cancel()
	resp, err := p.getOwnedInstance(ctx, id)
	if err != nil {
		return err
	}
	var volumes []string
	var page *string
	for {
		attachments, err := p.compute.ListBootVolumeAttachments(ociRequestContext(ctx), core.ListBootVolumeAttachmentsRequest{AvailabilityDomain: resp.AvailabilityDomain, CompartmentId: resp.CompartmentId, InstanceId: resp.Id, Page: page})
		if err != nil {
			return fmt.Errorf("inspect OCI boot-volume attachments: %w", err)
		}
		for _, attachment := range attachments.Items {
			if value(attachment.InstanceId) != id || value(attachment.CompartmentId) != p.profile.CompartmentID || value(attachment.BootVolumeId) == "" {
				return errors.New("OCI boot-volume attachment does not match the owned instance")
			}
			volumes = append(volumes, *attachment.BootVolumeId)
		}
		page = attachments.OpcNextPage
		if value(page) == "" {
			break
		}
	}
	if err := p.terminateOwnedInstance(ctx, resp); err != nil {
		return err
	}
	if err := pollOCI(ctx, p.pollInterval, func() (bool, error) {
		current, err := p.getOwnedInstance(ctx, id)
		if isOCINotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return current.LifecycleState == core.InstanceLifecycleStateTerminated, nil
	}); err != nil {
		return fmt.Errorf("wait for OCI instance deletion: %w", err)
	}
	for _, volumeID := range volumes {
		if err := pollOCI(ctx, p.pollInterval, func() (bool, error) {
			volume, err := p.block.GetBootVolume(ociRequestContext(ctx), core.GetBootVolumeRequest{BootVolumeId: common.String(volumeID)})
			if isOCINotFound(err) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			if value(volume.Id) != volumeID || value(volume.CompartmentId) != p.profile.CompartmentID {
				return false, errors.New("OCI boot-volume response does not match the owned instance's volume")
			}
			return volume.LifecycleState == core.BootVolumeLifecycleStateTerminated, nil
		}); err != nil {
			return fmt.Errorf("wait for OCI boot-volume deletion: %w", err)
		}
	}
	return nil
}
