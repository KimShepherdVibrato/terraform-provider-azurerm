package azurerm

import (
	"fmt"
	"log"
	"regexp"

	"github.com/Azure/azure-sdk-for-go/services/datafactory/mgmt/2018-06-01/datafactory"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func resourceArmDataFactoryIntegrationRuntime() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmDataFactoryIntegrationRuntimeCreateOrUpdate,
		Read:   resourceArmDataFactoryIntegrationRuntimeRead,
		Update: resourceArmDataFactoryIntegrationRuntimeCreateOrUpdate,
		Delete: resourceArmDataFactoryIntegrationRuntimeDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateAzureRMDataFactoryIntegrationRuntimeName,
			},

			"data_factory_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.StringMatch(
					regexp.MustCompile(`^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$`),
					`Invalid data_factory_name, see https://docs.microsoft.com/en-us/azure/data-factory/naming-rules`,
				),
			},

			// There's a bug in the Azure API where this is returned in lower-case
			// BUG: https://github.com/Azure/azure-rest-api-specs/issues/5788
			"resource_group_name": azure.SchemaResourceGroupNameDiffSuppress(),

			"type": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice([]string{"SelfHosted", "Managed"}, false),
			},

			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"auth_key_1": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"auth_key_2": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceArmDataFactoryIntegrationRuntimeCreateOrUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).dataFactory.IntegrationRuntimesClient
	ctx := meta.(*ArmClient).StopContext

	resourceGroupName := d.Get("resource_group_name").(string)
	name := d.Get("name").(string)
	dataFactoryName := d.Get("data_factory_name").(string)

	if requireResourcesToBeImported && d.IsNewResource() {
		existing, err := client.Get(ctx, resourceGroupName, dataFactoryName, name, "")
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("Error checking for presence of existing Data Factory Integration Runtime %q (Resource Group %q / Data Factory %q): %s", name, resourceGroupName, dataFactoryName, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_data_factory_integration_runtime", *existing.ID)
		}
	}

	description := d.Get("description").(string)

	var integrationRuntime datafactory.BasicIntegrationRuntime
	switch irType := d.Get("type").(string); irType {
	case "SelfHosted":
		integrationRuntime = &datafactory.SelfHostedIntegrationRuntime{
			Description: &description,
			Type:        datafactory.TypeSelfHosted,
		}
	case "Managed":
		integrationRuntime = &datafactory.ManagedIntegrationRuntime{
			Description: &description,
			Type:        datafactory.TypeManaged,
		}
	}

	config := datafactory.IntegrationRuntimeResource{
		Properties: integrationRuntime,
	}

	if _, err := client.CreateOrUpdate(ctx, resourceGroupName, dataFactoryName, name, config, ""); err != nil {
		return fmt.Errorf("Error creating Data Factory Integration Runtime %q (Resource Group %q / Data Factory %q): %+v", name, resourceGroupName, dataFactoryName, err)
	}

	read, err := client.Get(ctx, resourceGroupName, dataFactoryName, name, "")
	if err != nil {
		return fmt.Errorf("Error retrieving Data Factory Integration Runtime %q (Resource Group %q / Data Factory %q): %+v", name, resourceGroupName, dataFactoryName, err)
	}

	if read.ID == nil {
		return fmt.Errorf("Cannot read Data Factory Integration Runtime %q (Resource Group %q / Data Factory %q) ID", name, resourceGroupName, dataFactoryName)
	}

	d.SetId(*read.ID)

	return resourceArmDataFactoryIntegrationRuntimeRead(d, meta)
}

func resourceArmDataFactoryIntegrationRuntimeRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).dataFactory.IntegrationRuntimesClient
	ctx := meta.(*ArmClient).StopContext

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	dataFactoryName := id.Path["factories"]
	name := id.Path["integrationruntimes"]

	resp, err := client.Get(ctx, id.ResourceGroup, dataFactoryName, name, "")
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			d.SetId("")
			log.Printf("[DEBUG] Data Factory Integration Runtime %q was not found in Resource Group %q - removing from state!", name, id.ResourceGroup)
			return nil
		}
		return fmt.Errorf("Error reading the state of Data Factory Integration Runtime %q: %+v", name, err)
	}

	d.Set("name", resp.Name)
	d.Set("resource_group_name", id.ResourceGroup)
	d.Set("data_factory_name", dataFactoryName)

	if props, _ := resp.Properties.AsIntegrationRuntime(); props != nil {
		d.Set("description", props.Description)
		d.Set("type", props.Type)

		// Get the auth keys for a self hosted runtime
		if props.Type == "SelfHosted" {
			keys, err := client.ListAuthKeys(ctx, id.ResourceGroup, dataFactoryName, name)
			if err != nil {
				return err
			}
			d.Set("auth_key_1", keys.AuthKey1)
			d.Set("auth_key_2", keys.AuthKey2)
		}
	}

	return nil
}

func resourceArmDataFactoryIntegrationRuntimeDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).dataFactory.IntegrationRuntimesClient
	ctx := meta.(*ArmClient).StopContext

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	dataFactoryName := id.Path["factories"]
	name := id.Path["integrationruntimes"]
	resourceGroupName := id.ResourceGroup

	if _, err = client.Delete(ctx, resourceGroupName, dataFactoryName, name); err != nil {
		return fmt.Errorf("Error deleting Data Factory Integration Runtime %q (Resource Group %q / Data Factory %q): %+v", name, resourceGroupName, dataFactoryName, err)
	}

	return nil
}

func validateAzureRMDataFactoryIntegrationRuntimeName(v interface{}, k string) (warnings []string, errors []error) {
	value := v.(string)
	if regexp.MustCompile(`^[.+?/<>*%&:\\]+$`).MatchString(value) {
		errors = append(errors, fmt.Errorf("any of '.', '+', '?', '/', '<', '>', '*', '%%', '&', ':', '\\', are not allowed in %q: %q", k, value))
	}

	return warnings, errors
}
