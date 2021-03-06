package main

import (
	"context"
	"errors"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/scalechamp/goss"
	"go.uber.org/multierr"
	"time"
)

func resourceInstance(kind string) *schema.Resource {
	s := map[string]*schema.Schema{
		"name": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "Name of the instance",
		},
		"plan": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "Name of the plan, check in pricing",
		},
		"cloud": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "Name of the cloud",
		},
		"enabled": {
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     true,
			Description: "use in case do disabled service or enabled it from down",
		},
		"eviction_policy": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "redis|keydb|keydb-pro eviction policy",
		},
		"whitelist": {
			Type: schema.TypeSet,
			Optional:    true,
			Description: "IP whitelist set of strings",
			Elem: &schema.Schema{
				Type: schema.TypeString,
			},
		},
		"region": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "Name of the cloud region, see pricing table",
		},
		"master_host": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Hostname of master node",
		},
		"replica_host": {
			Type:        schema.TypeString,
			Computed:    true,
			Optional:    true,
			Description: "Hostname of replica node",
		},
		"password": {
			Type:        schema.TypeString,
			Optional:    true,
			Computed:    true,
			Description: "Password generated by ScaleChamp or provided by user",
		},
	}
	if kind == "keydb-pro" {
		s["license_key"] = &schema.Schema{
			Type:        schema.TypeString,
			Required:    true,
			Description: "Name of the instanceResource",
		}
	}
	if kind == "keydb" || kind == "keydb-pro" || kind == "redis" {
		s["eviction_policy"] = &schema.Schema{
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Name of the instanceResource",
		}
	}
	i := &instanceResource{kind}
	return &schema.Resource{
		Create: i.resourceCreate,
		Read:   i.resourceRead,
		Update: i.resourceUpdate,
		Delete: i.resourceDelete,
		Schema: s,
	}
}

type instanceResource struct {
	kind string
}

func (r *instanceResource) resourceCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*goss.Client)
	planFindRequest := &goss.PlanFindRequest{
		Cloud:  d.Get("cloud").(string),
		Region: d.Get("region").(string),
		Name:   d.Get("plan").(string),
		Kind:   r.kind,
	}
	plan, err := client.Plans.Find(context.TODO(), planFindRequest)
	if err != nil {
		return err
	}
	createRequest := &goss.InstanceCreateRequest{
		Name:      d.Get("name").(string),
		Whitelist: toStrings(d.Get("whitelist").(*schema.Set).List()),
		Password:  d.Get("password").(string),
		PlanID:    plan.ID,
	}
	if r.kind == "redis" || r.kind == "keydb" || r.kind == "keydb-pro" {
		createRequest.EvictionPolicy = d.Get("eviction_policy").(string)
	}
	if r.kind == "keydb-pro" {
		createRequest.LicenseKey = d.Get("license_key").(string)
	}
	data, err := client.Instances.Create(context.TODO(), createRequest)
	if err != nil {
		return err
	}
	d.SetId(data.ID)

	for i := 0; i < 36; i += 1 {
		time.Sleep(10 * time.Second)
		data, err = client.Instances.Get(context.TODO(), data.ID)
		if err != nil {
			return err
		}
		if data.State == "running" {
			break
		}
		if data.State == "failed" {
			return errors.New("failed instanceResource")
		}
	}

	return r.resourceRead(d, meta)
}

func toStrings(x []interface{}) []string {
	ips := make([]string, len(x))
	for i, v := range x {
		ips[i] = v.(string)
	}
	return ips
}

func (r *instanceResource) resourceUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*goss.Client)
	instanceUpdateRequest := new(goss.InstanceUpdateRequest)
	instanceUpdateRequest.ID = d.Id()

	if d.HasChange("name") {
		instanceUpdateRequest.Name = d.Get("name").(string)
	}

	if d.HasChange("cloud") || d.HasChange("region") || d.HasChange("plan") {
		planFindRequest := &goss.PlanFindRequest{
			Cloud:  d.Get("cloud").(string),
			Region: d.Get("region").(string),
			Name:   d.Get("plan").(string),
			Kind:   r.kind,
		}
		plan, err := client.Plans.Find(context.TODO(), planFindRequest)
		if err != nil {
			return err
		}
		instanceUpdateRequest.PlanID = plan.ID
	}

	if d.HasChange("password") {
		instanceUpdateRequest.Password = d.Get("password").(string)
	}

	if d.HasChange("enabled") {
		instanceUpdateRequest.Enabled = boolPtr(d.Get("enabled").(bool))
	}

	if d.HasChange("whitelist") {
		instanceUpdateRequest.Whitelist = toStrings(d.Get("whitelist").(*schema.Set).List())
	}

	if r.kind == "keydb" || r.kind == "redis" || r.kind == "keydb-pro" {
		if d.HasChange("eviction_policy") {
			instanceUpdateRequest.EvictionPolicy = d.Get("eviction_policy").(string)
		}
		if r.kind == "keydb-pro" {
			if d.HasChange("license_key") {
				instanceUpdateRequest.LicenseKey = d.Get("license_key").(string)
			}
		}
	}

	_, err := client.Instances.Update(context.TODO(), instanceUpdateRequest)
	if err != nil {
		return err
	}
	for i := 0; i < 50; i += 1 {
		time.Sleep(5 * time.Second)
		instance, err := client.Instances.Get(context.TODO(), d.Id())
		if err != nil {
			return err
		}
		if instance.State == "running" {
			break
		}
		if instance.State == "failed" {
			return errors.New("failed instanceResource")
		}
	}
	return r.resourceRead(d, meta)
}

func boolPtr(b bool) *bool {
	return &b
}

func (r *instanceResource) resourceRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*goss.Client)
	instance, err := client.Instances.Get(context.TODO(), d.Id())
	if err != nil {
		return err
	}
	return multierr.Combine(
		d.Set("password", instance.Password),
		d.Set("replica_host", instance.ConnectionInfo.ReplicaHost),
		d.Set("master_host", instance.ConnectionInfo.MasterHost),
	)
}

func (r *instanceResource) resourceDelete(d *schema.ResourceData, meta interface{}) error {
	api := meta.(*goss.Client)
	return api.Instances.Delete(context.TODO(), d.Id())
}
