package terraformutils

import "github.com/GoogleCloudPlatform/terraformer/terraformutils/terraformoutput"

// ModuleOutputGenerator is now just a wrapper around terraformoutput.ModuleOutputGenerator
type ModuleOutputGenerator struct {
	*terraformoutput.ModuleOutputGenerator
}

// NewModuleOutputGenerator creates a new ModuleOutputGenerator
func NewModuleOutputGenerator(resources []Resource, outputPath, providerName string) *ModuleOutputGenerator {
	return &ModuleOutputGenerator{
		ModuleOutputGenerator: terraformoutput.NewModuleOutputGenerator(resources, outputPath, providerName),
	}
}
