package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func TestOCILaunchPayloadUsesOwnedIdentityAndEffectiveCPU(t *testing.T) {
	for _, shape := range []string{"VM.Standard.A1.Flex", "VM.Standard.E5.Flex"} {
		t.Run(shape, func(t *testing.T) {
			var launches atomic.Int32
			p := testComputeProvider(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "GET" && r.URL.Path == "/20160918/instances":
					if r.URL.Query().Get("displayName") != "test-instance" || r.URL.Query().Get("compartmentId") != "compartment" {
						t.Error("launch identity preflight is not scoped")
					}
					writeOCIJSON(w, 200, []core.Instance{})
				case r.Method == "GET" && r.URL.Path == "/20160918/subnets/subnet":
					writeOCIJSON(w, 200, testSubnet())
				case r.Method == "POST" && r.URL.Path == "/20160918/instances":
					launches.Add(1)
					var details core.LaunchInstanceDetails
					if err := json.NewDecoder(r.Body).Decode(&details); err != nil {
						t.Error(err)
					}
					if r.Header.Get("opc-retry-token") != "stable-token" || details.FreeformTags[operationTag] != "stable-token" || details.FreeformTags[accessTag] != "test" || details.FreeformTags[managedTag] != "true" || details.FreeformTags["infinity-iac-subnet"] != "subnet" {
						t.Error("launch is missing exact ownership or operation identity")
					}
					wantCPU := float32(3)
					if shape == "VM.Standard.E5.Flex" {
						wantCPU = 1.5
					}
					if details.ShapeConfig == nil || *details.ShapeConfig.Ocpus != wantCPU || *details.ShapeConfig.MemoryInGBs != 4 {
						t.Errorf("unexpected shape sizing: %+v", details.ShapeConfig)
					}
					userData, err := base64.StdEncoding.DecodeString(details.Metadata["user_data"])
					if err != nil || string(userData) != "#!/bin/bash\ntrue\n" || details.Metadata["ssh_authorized_keys"] != "public-test-key" {
						t.Error("metadata was not preserved and encoded")
					}
					source, ok := details.SourceDetails.(core.InstanceSourceViaImageDetails)
					if !ok || value(source.ImageId) != "image" || source.BootVolumeSizeInGBs == nil || *source.BootVolumeSizeInGBs != 50 {
						t.Error("launch did not enforce image and boot-volume minimum")
					}
					item := ownedTestInstance(core.InstanceLifecycleStateProvisioning)
					item.Shape = common.String(shape)
					writeOCIJSON(w, 200, item)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
					writeOCINotFound(w)
				}
			})
			p.profile.Shape = shape
			p.profile.SSHPublicKey = "public-test-key"
			item, err := p.CreateInstance(testOCIContext(t), CreateRequest{Spec: domain.ServerSpec{Name: "test-instance", Provider: "oracle", Access: "test", Region: "us-phoenix-1", OS: "ubuntu-24.04", CPU: 3, RAMGB: 4, StorageGB: 20, Port: 8080}, IdempotencyKey: "stable-token", UserData: "#!/bin/bash\ntrue\n"})
			if err != nil {
				t.Fatal(err)
			}
			if launches.Load() != 1 || item.CPU != 3 || item.StorageGB != 50 || item.Metadata[operationTag] != "stable-token" || item.Metadata[accessTag] != "test" || item.Port != 8080 {
				t.Fatalf("unexpected launch result: %+v", item)
			}
		})
	}
}

func TestOCILaunchCannotAdoptPersonalOrLegacySameName(t *testing.T) {
	for _, tags := range []map[string]string{nil, {managedTag: "true"}, {managedTag: "true", accessTag: "other", operationTag: "token"}} {
		var mutations atomic.Int32
		p := testComputeProvider(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				mutations.Add(1)
			}
			item := ownedTestInstance(core.InstanceLifecycleStateRunning)
			item.FreeformTags = tags
			writeOCIJSON(w, 200, []core.Instance{item})
		})
		_, err := p.CreateInstance(testOCIContext(t), CreateRequest{Spec: domain.ServerSpec{Name: "test-instance", Provider: "oracle", OS: "ubuntu-24.04", CPU: 1, RAMGB: 1, StorageGB: 50}, IdempotencyKey: "token"})
		if err == nil || mutations.Load() != 0 {
			t.Fatalf("foreign name was adopted or mutated: %v", err)
		}
	}
}

func TestOCILaunchExactOperationReturnsExistingInstance(t *testing.T) {
	var launches atomic.Int32
	p := testComputeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/20160918/instances":
			writeOCIJSON(w, 200, []core.Instance{ownedTestInstance(core.InstanceLifecycleStateRunning)})
		case r.Method == "GET" && r.URL.Path == "/20160918/vnicAttachments":
			writeOCIJSON(w, 200, []core.VnicAttachment{})
		default:
			launches.Add(1)
			writeOCINotFound(w)
		}
	})
	item, err := p.CreateInstance(testOCIContext(t), CreateRequest{Spec: domain.ServerSpec{Name: "test-instance", Provider: "oracle", OS: "ubuntu-24.04", CPU: 2, RAMGB: 4, StorageGB: 50}, IdempotencyKey: "test-token"})
	if err != nil || launches.Load() != 0 || item.ID != "instance" || item.Metadata[operationTag] != "test-token" {
		t.Fatalf("exact launch was not reconciled: %+v %v", item, err)
	}
}

func TestOCIInstanceMutationsRejectPersonalAndCrossProfile(t *testing.T) {
	for _, tags := range []map[string]string{nil, {managedTag: "true"}, {managedTag: "true", accessTag: "other"}} {
		var mutations atomic.Int32
		p := testComputeProvider(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				mutations.Add(1)
			}
			item := ownedTestInstance(core.InstanceLifecycleStateRunning)
			item.FreeformTags = tags
			writeOCIJSON(w, 200, item)
		})
		ctx := testOCIContext(t)
		for name, action := range map[string]func() error{
			"get":         func() error { _, err := p.GetInstance(ctx, "instance"); return err },
			"start":       func() error { _, err := p.StartInstance(ctx, "instance"); return err },
			"stop":        func() error { _, err := p.StopInstance(ctx, "instance"); return err },
			"restart":     func() error { _, err := p.RestartInstance(ctx, "instance"); return err },
			"delete":      func() error { return p.DeleteInstance(ctx, "instance") },
			"delete-wait": func() error { return p.DeleteInstanceAndWait(ctx, "instance") },
		} {
			if err := action(); !errors.Is(err, ErrOCIOwnership) {
				t.Errorf("%s accepted unowned instance: %v", name, err)
			}
		}
		if mutations.Load() != 0 {
			t.Fatal("unowned instance was mutated")
		}
	}
}

func TestOCIRestartStartsStoppedInstanceAndUsesETag(t *testing.T) {
	for _, state := range []core.InstanceLifecycleStateEnum{core.InstanceLifecycleStateRunning, core.InstanceLifecycleStateStopped} {
		t.Run(string(state), func(t *testing.T) {
			var actions atomic.Int32
			p := testComputeProvider(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "GET" && r.URL.Path == "/20160918/instances/instance":
					writeOCIJSON(w, 200, ownedTestInstance(state))
				case r.Method == "POST" && r.URL.Path == "/20160918/instances/instance":
					actions.Add(1)
					want := "SOFTRESET"
					if state == core.InstanceLifecycleStateStopped {
						want = "START"
					}
					if r.URL.Query().Get("action") != want || r.Header.Get("if-match") != "version-1" {
						t.Errorf("unsafe or incorrect action: %s %+v", r.URL.String(), r.Header)
					}
					writeOCIJSON(w, 200, ownedTestInstance(core.InstanceLifecycleStateStarting))
				case r.URL.Path == "/20160918/vnicAttachments":
					writeOCIJSON(w, 200, []core.VnicAttachment{})
				default:
					t.Error("unexpected request")
					writeOCINotFound(w)
				}
			})
			_, err := p.RestartInstance(testOCIContext(t), "instance")
			if err != nil || actions.Load() != 1 {
				t.Fatalf("restart failed: %v", err)
			}
		})
	}
}

func TestOCIInstanceListingPaginatesAndReturnsPrimaryIPAndOperation(t *testing.T) {
	var pages atomic.Int32
	p := testComputeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/20160918/instances":
			pages.Add(1)
			if r.URL.Query().Get("page") == "" {
				w.Header().Set("opc-next-page", "next")
				personal := ownedTestInstance(core.InstanceLifecycleStateRunning)
				personal.FreeformTags = nil
				writeOCIJSON(w, 200, []core.Instance{personal})
			} else {
				writeOCIJSON(w, 200, []core.Instance{ownedTestInstance(core.InstanceLifecycleStateRunning)})
			}
		case "/20160918/vnicAttachments":
			writeOCIJSON(w, 200, []core.VnicAttachment{{Id: common.String("attach"), InstanceId: common.String("instance"), VnicId: common.String("vnic"), LifecycleState: core.VnicAttachmentLifecycleStateAttached}})
		case "/20160918/vnics/vnic":
			writeOCIJSON(w, 200, core.Vnic{Id: common.String("vnic"), IsPrimary: common.Bool(true), PublicIp: common.String("192.0.2.10")})
		default:
			t.Error("unexpected request")
			writeOCINotFound(w)
		}
	})
	items, err := p.ListInstances(testOCIContext(t), Filter{})
	if err != nil || len(items) != 1 || pages.Load() != 2 {
		t.Fatalf("listing failed: %+v %v", items, err)
	}
	if items[0].PublicIP != "192.0.2.10" || items[0].OS != "ubuntu-24.04" || items[0].CPU != 2 || items[0].StorageGB != 50 || items[0].Metadata[operationTag] != "test-token" || items[0].Access != "test" {
		t.Fatalf("metadata was lost: %+v", items[0])
	}
}

func TestOCIWaitCancelsAndDoesNotTreatUnknown404AsDeletion(t *testing.T) {
	p := testComputeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/absent") {
			writeOCINotFound(w)
			return
		}
		writeOCIJSON(w, 200, ownedTestInstance(core.InstanceLifecycleStateProvisioning))
	})
	if _, err := p.WaitInstance(testOCIContext(t), "absent", domain.StatusDeleted); err == nil {
		t.Fatal("an unobserved 404 was reported as owned deletion")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.WaitInstance(ctx, "instance", domain.StatusRunning); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait did not honor cancellation: %v", err)
	}
}

func TestOCIPollCancelsWhileWaiting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	checks := 0
	err := pollOCI(ctx, time.Hour, func() (bool, error) {
		checks++
		return false, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) || checks != 1 {
		t.Fatalf("poll did not cancel its wait: checks=%d err=%v", checks, err)
	}
}

func TestOCIDeleteWaitVerifiesAllBootVolumesAndPreservesData(t *testing.T) {
	var deleted atomic.Bool
	var volumeReads atomic.Int32
	var attachmentPages atomic.Int32
	p := testComputeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/20160918/instances/instance":
			state := core.InstanceLifecycleStateRunning
			if deleted.Load() {
				state = core.InstanceLifecycleStateTerminated
			}
			writeOCIJSON(w, 200, ownedTestInstance(state))
		case r.URL.Path == "/20160918/bootVolumeAttachments":
			attachmentPages.Add(1)
			if r.URL.Query().Get("availabilityDomain") != "AD-1" || r.URL.Query().Get("compartmentId") != "compartment" || r.URL.Query().Get("instanceId") != "instance" {
				t.Error("attachment list was not scoped to the owned instance")
			}
			id := "boot-1"
			if r.URL.Query().Get("page") == "" {
				w.Header().Set("opc-next-page", "next")
			} else {
				id = "boot-2"
			}
			writeOCIJSON(w, 200, []core.BootVolumeAttachment{{Id: common.String("attachment"), BootVolumeId: common.String(id), InstanceId: common.String("instance"), CompartmentId: common.String("compartment")}})
		case r.Method == "DELETE" && r.URL.Path == "/20160918/instances/instance":
			if r.URL.Query().Get("preserveBootVolume") != "false" || r.URL.Query().Get("preserveDataVolumesCreatedAtLaunch") == "false" || r.Header.Get("if-match") != "version-1" {
				t.Error("unsafe volume deletion or missing ETag")
			}
			deleted.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/20160918/bootVolumes/"):
			volumeReads.Add(1)
			if r.Method != "GET" {
				t.Error("boot volume was explicitly deleted instead of verified")
			}
			writeOCINotFound(w)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			writeOCINotFound(w)
		}
	})
	if err := p.DeleteInstanceAndWait(testOCIContext(t), "instance"); err != nil {
		t.Fatal(err)
	}
	if !deleted.Load() || volumeReads.Load() != 2 || attachmentPages.Load() != 2 {
		t.Fatal("not all deletion states were confirmed")
	}
}

func TestOCIOperatingSystemAndArchitecture(t *testing.T) {
	p := testComputeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		writeOCIJSON(w, 200, core.Image{Id: common.String("image"), LifecycleState: core.ImageLifecycleStateAvailable, OperatingSystem: common.String("Canonical Ubuntu"), OperatingSystemVersion: common.String("24.04 Minimal aarch64")})
	})
	label, err := p.OperatingSystem(testOCIContext(t))
	if err != nil || label != "ubuntu-24.04" || p.Architecture() != "arm64" {
		t.Fatalf("incorrect image label: %s %v", label, err)
	}
	p.profile.Shape = "VM.Standard.E5.Flex"
	if p.Architecture() != "amd64" {
		t.Fatal("x86 shape was not identified")
	}
	p.profile.Shape = "VM.GPU.A10.1"
	if p.Architecture() != "" {
		t.Fatal("unknown GPU architecture was guessed")
	}
}
