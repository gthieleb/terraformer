package test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GoogleCloudPlatform/terraformer/terraformutils"
	"github.com/hashicorp/terraform/terraform"
	"github.com/stillya/testcontainers-keycloak"
	"github.com/stretchr/testify/assert"
)

var keycloakContainer *keycloak.KeycloakContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	
	// Setup Keycloak container
	keycloakContainer, err = keycloak.RunContainer(ctx,
		"keycloak/keycloak:24.0",
		keycloak.WithContextPath("/auth"),
		keycloak.WithAdminUsername("admin"),
		keycloak.WithAdminPassword("admin"),
	)
	if err != nil {
		panic(err)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	if err := keycloakContainer.Terminate(ctx); err != nil {
		panic(err)
	}

	os.Exit(code)
}

func TestKeycloakModuleGeneration(t *testing.T) {
	// Create temp directory for test output
	tmpDir, err := os.MkdirTemp("", "keycloak-module-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Prepare test resources
	resources := []terraformutils.Resource{
		{
			ResourceName: "test_realm",
			InstanceInfo: &terraform.InstanceInfo{
				Type: "keycloak_realm",
			},
			Item: map[string]interface{}{
				"realm": "test-realm",
				"enabled": true,
				"display_name": "Test Realm",
			},
		},
		{
			ResourceName: "test_user",
			InstanceInfo: &terraform.InstanceInfo{
				Type: "keycloak_user",
			},
			Item: map[string]interface{}{
				"realm_id": "test-realm",
				"username": "testuser",
				"email": "testuser@example.com",
				"enabled": true,
				"first_name": "Test",
				"last_name": "User",
			},
		},
	}

	// Initialize and run module generator
	moduleGenerator := terraformutils.NewModuleGenerator(resources, tmpDir)
	err = moduleGenerator.Generate()
	assert.NoError(t, err)

	// Verify main.tf content
	mainTfContent, err := os.ReadFile(filepath.Join(tmpDir, "main.tf"))
	assert.NoError(t, err)
	assert.Contains(t, string(mainTfContent), "resource \"keycloak_realm\"")
	assert.Contains(t, string(mainTfContent), "resource \"keycloak_user\"")

	// Verify variables.tf content
	varsTfContent, err := os.ReadFile(filepath.Join(tmpDir, "variables.tf"))
	assert.NoError(t, err)
	assert.Contains(t, string(varsTfContent), "variable \"keycloak_realm\"")
	assert.Contains(t, string(varsTfContent), "variable \"keycloak_user\"")

	// Verify tfvars content
	tfvarsContent, err := os.ReadFile(filepath.Join(tmpDir, "terraform.tfvars.json"))
	assert.NoError(t, err)

	var tfvars map[string]interface{}
	err = json.Unmarshal(tfvarsContent, &tfvars)
	assert.NoError(t, err)

	// Verify realm resource
	realmResource, ok := tfvars["test_realm"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "test-realm", realmResource["realm"])
	assert.Equal(t, true, realmResource["enabled"])
	assert.Equal(t, "Test Realm", realmResource["display_name"])

	// Verify user resource
	userResource, ok := tfvars["test_user"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "test-realm", userResource["realm_id"])
	assert.Equal(t, "testuser", userResource["username"])
	assert.Equal(t, "testuser@example.com", userResource["email"])
}
