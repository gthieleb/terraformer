package terraformoutput

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GoogleCloudPlatform/terraformer/terraformutils"
)

// ModuleOutputGenerator handles the generation of Terraform modules
type ModuleOutputGenerator struct {
	Resources      []terraformutils.Resource
	OutputPath     string
	ProviderName   string
	ResourceTypes  map[string]bool
	MaxTfvarsLines int
}

// NewModuleOutputGenerator creates a new ModuleOutputGenerator
func NewModuleOutputGenerator(resources []terraformutils.Resource, outputPath, providerName string) *ModuleOutputGenerator {
	resourceTypes := make(map[string]bool)
	for _, r := range resources {
		if r.InstanceInfo != nil {
			resourceType := r.InstanceInfo.Type
			resourceTypes[resourceType] = true
		}
	}

	return &ModuleOutputGenerator{
		Resources:      resources,
		OutputPath:     outputPath,
		ProviderName:   providerName,
		ResourceTypes:  resourceTypes,
		MaxTfvarsLines: 100, // Maximum lines for a single tfvars file before splitting
	}
}

// GenerateModuleOutput generates a Terraform module from the resources
func (g *ModuleOutputGenerator) GenerateModuleOutput() error {
	// Create module directory
	moduleDir := filepath.Join(g.OutputPath, g.ProviderName)
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		return fmt.Errorf("failed to create module directory: %w", err)
	}

	// Generate main.tf and variables.tf with OutputHclFiles
	if err := OutputHclFiles(
		g.Resources,
		terraformutils.ProviderGenerator(nil), // TODO: Pass actual provider if needed
		moduleDir,
		"",    // serviceName - empty for all services
		true,  // isCompact - generate single resources file
		"hcl", // output format
		true,  // sort
	); err != nil {
		return fmt.Errorf("failed to generate HCL files: %w", err)
	}

	// Generate terraform.tfvars.json with variable values
	if err := g.generateTfvars(moduleDir); err != nil {
		return fmt.Errorf("failed to generate terraform.tfvars.json: %w", err)
	}

	return nil
}

// generateTfvars generates the terraform.tfvars.json file with variable values
func (g *ModuleOutputGenerator) generateTfvars(moduleDir string) error {
	// Group resources by type
	resourcesByType := make(map[string]map[string]map[string]interface{})

	for _, r := range g.Resources {
		if r.InstanceInfo == nil {
			continue
		}

		resourceType := r.InstanceInfo.Type
		resourceName := r.ResourceName

		// Create a normalized variable name (replace hyphens with underscores)
		varName := strings.ReplaceAll(resourceType, "-", "_")

		// Initialize maps if needed
		if resourcesByType[varName] == nil {
			resourcesByType[varName] = make(map[string]map[string]interface{})
		}

		// Use the structured attributes from r.Item
		attributes := make(map[string]interface{})
		if r.Item != nil {
			attributes = r.Item
		}

		// Only add resources with attributes
		if len(attributes) > 0 {
			resourcesByType[varName][resourceName] = attributes
		}
	}

	// Generate main tfvars file
	tfvarsPath := filepath.Join(moduleDir, "terraform.tfvars.json")
	tfvarsData := make(map[string]interface{})

	// Check if any resource type has too many resources
	for resourceType, resources := range resourcesByType {
		// Count lines in JSON representation
		resourcesJSON, err := json.MarshalIndent(resources, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal resources: %w", err)
		}

		lines := len(strings.Split(string(resourcesJSON), "\n"))

		if lines > g.MaxTfvarsLines {
			// Create a separate file for this resource type
			resourceTfvarsPath := filepath.Join(moduleDir, fmt.Sprintf("resource_%s.tfvars.json", resourceType))
			resourceTfvarsData := map[string]interface{}{
				resourceType: resources,
			}

			resourceTfvarsJSON, err := json.MarshalIndent(resourceTfvarsData, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal resource tfvars: %w", err)
			}

			if err := os.WriteFile(resourceTfvarsPath, resourceTfvarsJSON, 0644); err != nil {
				return fmt.Errorf("failed to write resource tfvars: %w", err)
			}

			// Add a reference to the main tfvars file
			tfvarsData[resourceType] = fmt.Sprintf("See %s", filepath.Base(resourceTfvarsPath))
		} else {
			// Add to the main tfvars file
			tfvarsData[resourceType] = resources
		}
	}

	// Write main tfvars file
	tfvarsJSON, err := json.MarshalIndent(tfvarsData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tfvars: %w", err)
	}

	return os.WriteFile(tfvarsPath, tfvarsJSON, 0644)
}
