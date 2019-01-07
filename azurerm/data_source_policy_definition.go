package azurerm

import (
	"fmt"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/structure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/validate"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func dataSourceArmPolicyDefinition() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceArmPolicyDefinitionRead,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validate.NoEmptyStrings,
			},

			"policy_type": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"mode": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"management_group_id": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"display_name": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"description": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"policy_rule": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"metadata": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"parameters": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceArmPolicyDefinitionRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).policyDefinitionsClient
	ctx := meta.(*ArmClient).StopContext

	name := d.Get("name").(string)

	resp, err := client.Get(ctx, name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			return fmt.Errorf("Error: Policy Definition %q was not found", name)
		}
		return err
	}

	d.SetId(*resp.ID)

	if props := resp.DefinitionProperties; props != nil {
		d.Set("policy_type", props.PolicyType)
		d.Set("mode", props.Mode)
		d.Set("display_name", props.DisplayName)
		d.Set("description", props.Description)

		if policyRule := props.PolicyRule; policyRule != nil {
			policyRuleVal := policyRule.(map[string]interface{})
			policyRuleStr, err := structure.FlattenJsonToString(policyRuleVal)
			if err != nil {
				return fmt.Errorf("unable to flatten JSON for `policy_rule`: %s", err)
			}

			d.Set("policy_rule", policyRuleStr)
		}

		if metadata := props.Metadata; metadata != nil {
			metadataVal := metadata.(map[string]interface{})
			metadataStr, err := structure.FlattenJsonToString(metadataVal)
			if err != nil {
				return fmt.Errorf("unable to flatten JSON for `metadata`: %s", err)
			}

			d.Set("metadata", metadataStr)
		}

		if parameters := props.Parameters; parameters != nil {
			paramsVal := props.Parameters.(map[string]interface{})
			parametersStr, err := structure.FlattenJsonToString(paramsVal)
			if err != nil {
				return fmt.Errorf("unable to flatten JSON for `parameters`: %s", err)
			}

			d.Set("parameters", parametersStr)
		}
	}

	return nil
}
