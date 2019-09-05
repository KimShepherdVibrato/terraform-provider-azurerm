package azurerm

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-06-01/compute"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/structure"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/validate"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/features"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tags"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func resourceArmVirtualMachineExtensions() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmVirtualMachineExtensionsCreateUpdate,
		Read:   resourceArmVirtualMachineExtensionsRead,
		Update: resourceArmVirtualMachineExtensionsCreateUpdate,
		Delete: resourceArmVirtualMachineExtensionsDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"virtual_machine_id": {
				Type:          schema.TypeString,
				Optional:      true, //todo required in 2.0
				ForceNew:      true,
				ValidateFunc:  azure.ValidateResourceID,
				ConflictsWith: []string{"virtual_machine_name"},
			},

			"location": azure.SchemaLocation(),

			"resource_group_name": {
				Type:       schema.TypeString,
				Optional:   true,
				ForceNew:   true,
				Computed:   true,
				Deprecated: "This property has been deprecated as the resource group is now pulled from the virtual machine ID and will be removed in version 2.0 of the provider",
				ValidateFunc: func(v interface{}, k string) (warnings []string, errors []error) {
					value := v.(string)

					if len(value) > 80 {
						errors = append(errors, fmt.Errorf("%q may not exceed 80 characters in length", k))
					}

					if strings.HasSuffix(value, ".") {
						errors = append(errors, fmt.Errorf("%q may not end with a period", k))
					}

					// regex pulled from https://docs.microsoft.com/en-us/rest/api/resources/resourcegroups/createorupdate
					if matched := regexp.MustCompile(`^[-\w\._\(\)]+$`).Match([]byte(value)); !matched {
						errors = append(errors, fmt.Errorf("%q may only contain alphanumeric characters, dash, underscores, parentheses and periods", k))
					}

					return warnings, errors
				},
			},

			"virtual_machine_name": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				Deprecated:    "This property has been deprecated in favour of the virtual_machine_id property and will be removed in version 2.0 of the provider",
				ValidateFunc:  validate.NoEmptyStrings,
				ConflictsWith: []string{"virtual_machine_id"},
			},

			"publisher": {
				Type:     schema.TypeString,
				Required: true,
			},

			"type": {
				Type:     schema.TypeString,
				Required: true,
			},

			"type_handler_version": {
				Type:     schema.TypeString,
				Required: true,
			},

			"auto_upgrade_minor_version": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"settings": {
				Type:             schema.TypeString,
				Optional:         true,
				ValidateFunc:     validation.ValidateJsonString,
				DiffSuppressFunc: structure.SuppressJsonDiff,
			},

			// due to the sensitive nature, these are not returned by the API
			"protected_settings": {
				Type:             schema.TypeString,
				Optional:         true,
				Sensitive:        true,
				ValidateFunc:     validation.ValidateJsonString,
				DiffSuppressFunc: structure.SuppressJsonDiff,
			},

			"tags": tags.Schema(),
		},
	}
}

func resourceArmVirtualMachineExtensionsCreateUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).compute.VMExtensionClient
	ctx := meta.(*ArmClient).StopContext

	name := d.Get("name").(string)

	vmId := d.Get("virtual_machine_id").(string)
	vmName := d.Get("virtual_machine_name").(string)

	resGroup := d.Get("resource_group_name").(string)

	if features.ShouldResourcesBeImported() && d.IsNewResource() {
		existing, err := client.Get(ctx, resGroup, vmName, name, "")
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("Error checking for presence of existing Extension %q (Virtual Machine %q / Resource Group %q): %s", name, vmName, resGroup, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_virtual_machine_extension", *existing.ID)
		}
	}

	location := azure.NormalizeLocation(d.Get("location").(string))
	publisher := d.Get("publisher").(string)
	extensionType := d.Get("type").(string)
	typeHandlerVersion := d.Get("type_handler_version").(string)
	autoUpgradeMinor := d.Get("auto_upgrade_minor_version").(bool)
	t := d.Get("tags").(map[string]interface{})

	if vmName == "" {
		if vmId == "" {
			return fmt.Errorf("one of `virtual_machine_id` or `virtual_machine_name` must be set")
		}
		id, err2 := azure.ParseAzureResourceID(vmId)
		if err2 != nil {
			return err2
		}

		resGroup = id.ResourceGroup

		vmNameTemp, ok := id.Path["virtualMachines"]
		if !ok {
			return fmt.Errorf("virtual_machine_id does not contain `virtualMachines`: %q", vmId)
		}
		vmName = vmNameTemp

	} else if resGroup == "" {
		return fmt.Errorf("one of `resource_group_name` must be set when `virtual_machine_name` is used")
	}

	extension := compute.VirtualMachineExtension{
		Location: &location,
		VirtualMachineExtensionProperties: &compute.VirtualMachineExtensionProperties{
			Publisher:               &publisher,
			Type:                    &extensionType,
			TypeHandlerVersion:      &typeHandlerVersion,
			AutoUpgradeMinorVersion: &autoUpgradeMinor,
		},
		Tags: tags.Expand(t),
	}

	if settingsString := d.Get("settings").(string); settingsString != "" {
		settings, err := structure.ExpandJsonFromString(settingsString)
		if err != nil {
			return fmt.Errorf("unable to parse settings: %s", err)
		}
		extension.VirtualMachineExtensionProperties.Settings = &settings
	}

	if protectedSettingsString := d.Get("protected_settings").(string); protectedSettingsString != "" {
		protectedSettings, err := structure.ExpandJsonFromString(protectedSettingsString)
		if err != nil {
			return fmt.Errorf("unable to parse protected_settings: %s", err)
		}
		extension.VirtualMachineExtensionProperties.ProtectedSettings = &protectedSettings
	}

	future, err := client.CreateOrUpdate(ctx, resGroup, vmName, name, extension)
	if err != nil {
		return err
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return err
	}

	read, err := client.Get(ctx, resGroup, vmName, name, "")
	if err != nil {
		return err
	}

	if read.ID == nil {
		return fmt.Errorf("Cannot read  Virtual Machine Extension %s (resource group %s) ID", name, resGroup)
	}

	d.SetId(*read.ID)

	return resourceArmVirtualMachineExtensionsRead(d, meta)
}

func resourceArmVirtualMachineExtensionsRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).compute.VMExtensionClient
	ctx := meta.(*ArmClient).StopContext

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	resGroup := id.ResourceGroup
	vmName := id.Path["virtualMachines"]
	name := id.Path["extensions"]

	resp, err := client.Get(ctx, resGroup, vmName, name, "")

	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error making Read request on Virtual Machine Extension %s: %s", name, err)
	}

	d.Set("name", resp.Name)
	if location := resp.Location; location != nil {
		d.Set("location", azure.NormalizeLocation(*location))
	}

	d.Set("virtual_machine_id", id)

	d.Set("virtual_machine_name", vmName)
	d.Set("resource_group_name", resGroup)

	if props := resp.VirtualMachineExtensionProperties; props != nil {
		d.Set("publisher", props.Publisher)
		d.Set("type", props.Type)
		d.Set("type_handler_version", props.TypeHandlerVersion)
		d.Set("auto_upgrade_minor_version", props.AutoUpgradeMinorVersion)

		if settings := props.Settings; settings != nil {
			settingsVal := settings.(map[string]interface{})
			settingsJson, err := structure.FlattenJsonToString(settingsVal)
			if err != nil {
				return fmt.Errorf("unable to parse settings from response: %s", err)
			}
			d.Set("settings", settingsJson)
		}
	}

	return tags.FlattenAndSet(d, resp.Tags)
}

func resourceArmVirtualMachineExtensionsDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).compute.VMExtensionClient
	ctx := meta.(*ArmClient).StopContext

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	resGroup := id.ResourceGroup
	name := id.Path["extensions"]
	vmName := id.Path["virtualMachines"]

	future, err := client.Delete(ctx, resGroup, vmName, name)
	if err != nil {
		return err
	}

	return future.WaitForCompletionRef(ctx, client.Client)
}
