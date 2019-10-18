package opentelekomcloud

import (
	"bytes"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/hashcode"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"

	volumes_v2 "github.com/huaweicloud/golangsdk/openstack/blockstorage/v2/volumes"
	"github.com/huaweicloud/golangsdk/openstack/evs/v3/volumes"
)

func resourceEvsStorageVolumeV3() *schema.Resource {
	return &schema.Resource{
		Create: resourceEvsVolumeV3Create,
		Read:   resourceEvsVolumeV3Read,
		Update: resourceEvsVolumeV3Update,
		Delete: resourceBlockStorageVolumeV2Delete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(3 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"backup_id": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"availability_zone": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"size": {
				Type:     schema.TypeInt,
				Optional: true,
				ForceNew: true,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"snapshot_id": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"image_id": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"volume_type": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice([]string{
					"SATA", "SAS", "SSD", "co-p1", "uh-l1",
				}, true),
			},
			"tags": {
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: false,
			},
			"attachment": {
				Type:     schema.TypeSet,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"instance_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"device": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
				Set: resourceVolumeAttachmentHash,
			},
			"multiattach": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  false,
			},
			"cascade": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  true,
			},
		},
	}
}

func resourceVolumeAttachmentHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})
	if m["instance_id"] != nil {
		buf.WriteString(fmt.Sprintf("%s-", m["instance_id"].(string)))
	}
	return hashcode.String(buf.String())
}

func resourceEvsVolumeV3Create(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	blockStorageClient, err := config.blockStorageV3Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenTelekomCloud EVS storage client: %s", err)
	}

	if !hasFilledOpt(d, "backup_id") && !hasFilledOpt(d, "size") {
		return fmt.Errorf("Missing required argument: 'size' is required, but no definition was found.")
	}
	tags := resourceContainerTags(d)
	createOpts := &volumes.CreateOpts{
		BackupID:         d.Get("backup_id").(string),
		AvailabilityZone: d.Get("availability_zone").(string),
		Description:      d.Get("description").(string),
		Size:             d.Get("size").(int),
		Name:             d.Get("name").(string),
		SnapshotID:       d.Get("snapshot_id").(string),
		ImageRef:         d.Get("image_id").(string),
		VolumeType:       d.Get("volume_type").(string),
		Multiattach:      d.Get("multiattach").(bool),
		Tags:             tags,
	}

	log.Printf("[DEBUG] Create Options: %#v", createOpts)
	v, err := volumes.Create(blockStorageClient, createOpts).ExtractJobResponse()
	if err != nil {
		return fmt.Errorf("Error creating OpenTelekomCloud EVS volume: %s", err)
	}
	log.Printf("[INFO] Volume Job ID: %s", v.JobID)

	// Wait for the volume to become available.
	log.Printf("[DEBUG] Waiting for volume to become available")
	err = volumes.WaitForJobSuccess(blockStorageClient, int(d.Timeout(schema.TimeoutCreate)/time.Second), v.JobID)
	if err != nil {
		return err
	}

	entity, err := volumes.GetJobEntity(blockStorageClient, v.JobID, "volume_id")
	if err != nil {
		return err
	}

	if id, ok := entity.(string); ok {
		log.Printf("[INFO] Volume ID: %s", id)
		// Store the ID now
		d.SetId(id)
		return resourceEvsVolumeV3Read(d, meta)
	}
	return fmt.Errorf("Unexpected conversion error in resourceEvsVolumeV3Create.")
}

func resourceEvsVolumeV3Read(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	blockStorageClient, err := config.blockStorageV3Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenTelekomCloud EVS storage client: %s", err)
	}

	v, err := volumes.Get(blockStorageClient, d.Id()).Extract()
	if err != nil {
		return CheckDeleted(d, err, "volume")
	}

	log.Printf("[DEBUG] Retrieved volume %s: %+v", d.Id(), v)

	d.Set("size", v.Size)
	d.Set("description", v.Description)
	d.Set("availability_zone", v.AvailabilityZone)
	d.Set("name", v.Name)
	d.Set("snapshot_id", v.SnapshotID)
	d.Set("source_vol_id", v.SourceVolID)
	d.Set("volume_type", v.VolumeType)

	// set tags
	tags := make(map[string]string)
	for key, val := range v.Tags {
		tags[key] = val
	}
	if err := d.Set("tags", tags); err != nil {
		return fmt.Errorf("[DEBUG] Error saving tags to state for OpenTelekomCloud evs storage (%s): %s", d.Id(), err)
	}

	// set attachments
	attachments := make([]map[string]interface{}, len(v.Attachments))
	for i, attachment := range v.Attachments {
		attachments[i] = make(map[string]interface{})
		attachments[i]["id"] = attachment.ID
		attachments[i]["instance_id"] = attachment.ServerID
		attachments[i]["device"] = attachment.Device
		log.Printf("[DEBUG] attachment: %v", attachment)
	}
	if err := d.Set("attachment", attachments); err != nil {
		return fmt.Errorf("[DEBUG] Error saving attachment to state for OpenTelekomCloud evs storage (%s): %s", d.Id(), err)
	}

	return nil
}

// using OpenStack Cinder API v2 to update volume resource
func resourceEvsVolumeV3Update(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	blockStorageClient, err := config.loadEVSV2Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating OpenTelekomCloud block storage client: %s", err)
	}

	updateOpts := volumes_v2.UpdateOpts{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
	}

	_, err = volumes_v2.Update(blockStorageClient, d.Id(), updateOpts).Extract()
	if err != nil {
		return fmt.Errorf("Error updating OpenTelekomCloud volume: %s", err)
	}

	if d.HasChange("tags") {
		_, err = resourceEVSTagV2Create(d, meta, "volumes", d.Id(), resourceContainerTags(d))
	}
	return resourceEvsVolumeV3Read(d, meta)
}