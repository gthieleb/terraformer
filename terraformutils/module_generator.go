package terraformutils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type ModuleGenerator struct {
	resources []Resource
	path      string
}

func NewModuleGenerator(resources []Resource, outputPath string) *ModuleGenerator {
	return &ModuleGenerator{
		resources: resources,
		path:      outputPath,
	}
}

func (g *ModuleGenerator) Generate() error {
	if err := os.MkdirAll(g.path, 0755); err != nil {
		return err
	}

	resourcesByType := g.groupResourcesByType()
	
	if err := g.generateMainTF(resourcesByType); err != nil {
		return err
	}

	if err := g.generateVariablesTF(resourcesByType); err != nil {
		return err
	}

	return g.generateTFVars(resourcesByType)
}

func (g *ModuleGenerator) groupResourcesByType() map[string][]Resource {
	result := make(map[string][]Resource)
	for _, r := range g.resources {
		resourceType := r.InstanceInfo.Type
		result[resourceType] = append(result[resourceType], r)
	}
	return result
}

func (g *ModuleGenerator) generateMainTF(resourcesByType map[string][]Resource) error {
	f, err := os.Create(filepath.Join(g.path, "main.tf"))
	if err != nil {
		return err
	}
	defer f.Close()

	for resourceType, resources := range resourcesByType {
		if len(resources) == 0 {
			continue
		}

		// Get common attributes across all resources of this type
		commonAttrs := g.getCommonAttributes(resources)
		
		// Generate the resource template
		template := fmt.Sprintf(`
resource "%s" "this" {
  for_each = var.%s

  # Common attributes
%s

  # Dynamic attributes
  dynamic "settings" {
    for_each = each.value
    content {
      for_each = settings.value
      content {
        name  = content.key
        value = content.value
      }
    }
  }
}
`, resourceType, g.getVariableName(resourceType), g.formatCommonAttrs(commonAttrs))

		if _, err := f.WriteString(template); err != nil {
			return err
		}
	}
	return nil
}

func (g *ModuleGenerator) generateVariablesTF(resourcesByType map[string][]Resource) error {
	f, err := os.Create(filepath.Join(g.path, "variables.tf"))
	if err != nil {
		return err
	}
	defer f.Close()

	for resourceType := range resourcesByType {
		varDef := fmt.Sprintf(`
variable "%s" {
  description = "Map of %s resources"
  type        = map(any)
}
`, g.getVariableName(resourceType), resourceType)

		if _, err := f.WriteString(varDef); err != nil {
			return err
		}
	}
	return nil
}

func (g *ModuleGenerator) generateTFVars(resourcesByType map[string][]Resource) error {
	for resourceType, resources := range resourcesByType {
		tfvars := make(map[string]interface{})
		
		for _, resource := range resources {
			// Remove any empty or nil values
			cleanedItems := g.cleanResourceItems(resource.Item)
			if len(cleanedItems) > 0 {
				tfvars[resource.ResourceName] = cleanedItems
			}
		}

		// Determine output file based on size
		var filename string
		if len(tfvars) > 100 {
			filename = fmt.Sprintf("%s.tfvars.json", resourceType)
		} else {
			filename = "terraform.tfvars.json"
		}

		// Write to file
		f, err := os.Create(filepath.Join(g.path, filename))
		if err != nil {
			return err
		}
		
		if err := json.NewEncoder(f).Encode(tfvars); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}

func (g *ModuleGenerator) getVariableName(resourceType string) string {
	return strings.Replace(resourceType, "-", "_", -1)
}

func (g *ModuleGenerator) getCommonAttributes(resources []Resource) map[string]interface{} {
	if len(resources) == 0 {
		return nil
	}

	// Start with all attributes from first resource
	common := make(map[string]interface{})
	for k, v := range resources[0].Item {
		common[k] = v
	}

	// Remove attributes that differ across resources
	for _, resource := range resources[1:] {
		for k, v := range common {
			if resourceVal, exists := resource.Item[k]; !exists || !reflect.DeepEqual(v, resourceVal) {
				delete(common, k)
			}
		}
	}

	return common
}

func (g *ModuleGenerator) formatCommonAttrs(attrs map[string]interface{}) string {
	var result strings.Builder
	for k, v := range attrs {
		result.WriteString(fmt.Sprintf("  %s = %v\n", k, v))
	}
	return result.String()
}

func (g *ModuleGenerator) cleanResourceItems(items map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range items {
		if !g.isEmpty(v) {
			result[k] = v
		}
	}
	return result
}

func (g *ModuleGenerator) isEmpty(v interface{}) bool {
	if v == nil {
		return true
	}
	
	switch value := v.(type) {
	case string:
		return value == ""
	case []interface{}:
		return len(value) == 0
	case map[string]interface{}:
		return len(value) == 0
	default:
		return false
	}
}
