package azurerm

import (
	"fmt"
	"log"
	"regexp"

	"github.com/Azure/azure-sdk-for-go/services/datafactory/mgmt/2018-06-01/datafactory"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/suppress"
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

			"compute_properties": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{

						"location": azure.SchemaLocation(),

						"node_size": {
							Type:             schema.TypeString,
							Required:         true,
							DiffSuppressFunc: suppress.CaseDifference,
						},

						"node_count": {
							Type:         schema.TypeInt,
							Required:     true,
							ValidateFunc: validation.IntBetween(2, 8),
						},

						"max_node_executions": {
							Type:         schema.TypeInt,
							Required:     true,
							ValidateFunc: validation.IntBetween(2, 8),
						},

						"vnet_id": {
							Type:             schema.TypeString,
							Optional:         true,
							DiffSuppressFunc: suppress.CaseDifference,
							ValidateFunc:     azure.ValidateResourceID,
						},

						"subnet": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},

			"ssis_properties": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{

						"catalog_info": ,
							"CatalogServerEndpoint"
							"CatalogAdminUserName"
							"CatalogAdminPassword"
							"CatalogPricingTier"
						"custom_setup_script_properties": ,
							"BlobContainerURI"
							"SasToken"
						"data_proxy_properties": ,
							"ConnectVia"
								"ReferenceName"
								"***Type"
							"StagingLinkedService"
								"ReferenceName"
								"***Type"
							"Path"
						"edition": ,
						"licenseType": {"BasePrice", "LicenseIncluded"},
					},
				},
			},

			"auth_key_1": {
				Type:      schema.TypeString,
				Computed:  true,
				Sensitive: true,
			},

			"auth_key_2": {
				Type:      schema.TypeString,
				Computed:  true,
				Sensitive: true,
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
	case string(datafactory.TypeSelfHosted):
		integrationRuntime = &datafactory.SelfHostedIntegrationRuntime{
			Description: &description,
			Type:        datafactory.TypeSelfHosted,
		}
	case string(datafactory.TypeManaged):
		managedIntegrationRuntimeComputeProperties, err := expandAzureDataFactoryIntegrationRuntimeComputeProperties(d)
		if err != nil {
			return fmt.Errorf("Error parsing integration runtime compute properties: %s", err)
		}
		managedIntegrationRuntimeProperties := &datafactory.ManagedIntegrationRuntimeTypeProperties{
			ComputeProperties: managedIntegrationRuntimeComputeProperties,
		}
		integrationRuntime = &datafactory.ManagedIntegrationRuntime{
			ManagedIntegrationRuntimeTypeProperties: managedIntegrationRuntimeProperties,
			Description:                             &description,
			Type:                                    datafactory.TypeManaged,
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

		switch props.Type {
		case datafactory.TypeSelfHosted:
			keys, err := client.ListAuthKeys(ctx, id.ResourceGroup, dataFactoryName, name)
			if err != nil {
				return err
			}
			d.Set("auth_key_1", keys.AuthKey1)
			d.Set("auth_key_2", keys.AuthKey2)

		case datafactory.TypeManaged:
			managedIntegrationRuntime, _ := resp.Properties.AsManagedIntegrationRuntime()
			if err := d.Set("compute_properties", flattenAzureDataFactoryIntegrationRuntimeComputeProperties(managedIntegrationRuntime.ManagedIntegrationRuntimeTypeProperties.ComputeProperties)); err != nil {
				return fmt.Errorf("Error flattening `compute_properties`: %+v", err)
			}
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

func flattenAzureDataFactoryIntegrationRuntimeComputeProperties(properties *datafactory.IntegrationRuntimeComputeProperties) interface{} {
	if properties == nil {
		return make([]interface{}, 0)
	}

	result := make(map[string]interface{})
	if properties.Location != nil {
		result["location"] = *properties.Location
	}
	if properties.NodeSize != nil {
		result["node_size"] = *properties.NodeSize
	}
	if properties.NumberOfNodes != nil {
		result["node_count"] = *properties.NumberOfNodes
	}
	if properties.MaxParallelExecutionsPerNode != nil {
		result["max_node_executions"] = *properties.MaxParallelExecutionsPerNode
	}
	if properties.VNetProperties != nil {
		result["vnet_id"] = *properties.VNetProperties.VNetID
		result["subnet"] = *properties.VNetProperties.Subnet
	}

	return []interface{}{result}
}

func expandAzureDataFactoryIntegrationRuntimeComputeProperties(d *schema.ResourceData) (*datafactory.IntegrationRuntimeComputeProperties, error) {
	computeProperties := d.Get("compute_properties").([]interface{})
	config := computeProperties[0].(map[string]interface{})

	location := config["location"].(string)
	nodeSize := config["node_size"].(string)
	nodeCount := int32(config["node_count"].(int))
	maxNodeExecutions := int32(config["max_node_executions"].(int))

	vnetID := config["vnet_id"].(string)
	subnet := config["subnet"].(string)

	var integrationRuntimeVNetProperties *datafactory.IntegrationRuntimeVNetProperties
	if vnetID != "" && subnet != "" {
		integrationRuntimeVNetProperties = &datafactory.IntegrationRuntimeVNetProperties{
			VNetID: &vnetID,
			Subnet: &subnet,
		}
	} else if vnetID != "" || subnet != "" {
		return nil, fmt.Errorf("Both `vnet_id` and `subnet` must be provided if setting the vnet properties")
	} else {
		integrationRuntimeVNetProperties = nil
	}

	integrationRuntimeComputeProperties := &datafactory.IntegrationRuntimeComputeProperties{
		Location:                     &location,
		NodeSize:                     &nodeSize,
		NumberOfNodes:                &nodeCount,
		MaxParallelExecutionsPerNode: &maxNodeExecutions,
		VNetProperties:               integrationRuntimeVNetProperties,
	}

	return integrationRuntimeComputeProperties, nil
}
