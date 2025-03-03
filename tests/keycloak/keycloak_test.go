package keycloak_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/terraformer/cmd"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	keycloakImage      = "quay.io/keycloak/keycloak:22.0.1"
	keycloakUsername   = "admin"
	keycloakPassword   = "admin"
	keycloakPort       = "8080/tcp"
	keycloakRealm      = "test-realm"
	keycloakClientID   = "test-client"
	keycloakClientName = "Test Client"
	keycloakRoleID     = "test-role"
	keycloakRoleName   = "Test Role"
)

type keycloakContainer struct {
	testcontainers.Container
	URI      string
	Username string
	Password string
}

// setupKeycloak creates and starts a Keycloak container
func setupKeycloak(ctx context.Context) (*keycloakContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        keycloakImage,
		ExposedPorts: []string{keycloakPort},
		Env: map[string]string{
			"KEYCLOAK_ADMIN":          keycloakUsername,
			"KEYCLOAK_ADMIN_PASSWORD": keycloakPassword,
			"KC_HEALTH_ENABLED":       "true",
			"KC_METRICS_ENABLED":      "true",
			"KC_FEATURES":             "token-exchange,admin-fine-grained-authz",
			"KC_DB":                   "dev-mem",
		},
		Cmd: []string{"start-dev"},
		WaitingFor: wait.ForAll(
			wait.ForLog("Running the server in development mode"),
			wait.ForHTTP("/health/ready").WithPort(nat.Port(keycloakPort)).WithStatusCodeMatcher(func(status int) bool {
				return status == 200
			}).WithStartupTimeout(2 * time.Minute),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Reuse:            false, // Set to false to create a new container for each test run
	})
	if err != nil {
		return nil, err
	}

	ip, err := container.Host(ctx)
	if err != nil {
		return nil, err
	}

	mappedPort, err := container.MappedPort(ctx, nat.Port(keycloakPort))
	if err != nil {
		return nil, err
	}

	uri := fmt.Sprintf("http://%s:%s", ip, mappedPort.Port())

	return &keycloakContainer{
		Container: container,
		URI:       uri,
		Username:  keycloakUsername,
		Password:  keycloakPassword,
	}, nil
}

// createTestRealm creates a test realm and client in Keycloak
func createTestRealm(ctx context.Context, kc *keycloakContainer) error {
	// Add a longer delay to ensure Keycloak is fully initialized
	time.Sleep(15 * time.Second)

	// Get container logs for debugging
	logReader, err := kc.Container.Logs(ctx)
	if err == nil {
		defer logReader.Close()
		logBytes, _ := ioutil.ReadAll(logReader)
		fmt.Println("Container logs:", string(logBytes))
	}

	// First authenticate with Keycloak using internal URL
	// When executing commands inside the container, we need to use http://localhost:8080
	authCmd := []string{
		"sh", "-c",
		"/opt/keycloak/bin/kcadm.sh config credentials --server http://localhost:8080 --realm master --user admin --password admin",
	}

	exitCode, authOutputReader, err := kc.Exec(ctx, authCmd)
	if err != nil || exitCode != 0 {
		authOutput, _ := ioutil.ReadAll(authOutputReader)
		return fmt.Errorf("failed to authenticate with Keycloak: %v, exit code: %d, output: %s", 
			err, exitCode, string(authOutput))
	}

	// Create realm - no need to specify server, user, password again as we're already authenticated
	realmCmd := []string{
		"sh", "-c",
		fmt.Sprintf("/opt/keycloak/bin/kcadm.sh create realms -s realm=%s -s enabled=true", 
			keycloakRealm),
	}

	exitCode, realmOutputReader, err := kc.Exec(ctx, realmCmd)
	if err != nil || exitCode != 0 {
		realmOutput, _ := ioutil.ReadAll(realmOutputReader)
		return fmt.Errorf("failed to create realm: %v, exit code: %d, output: %s", 
			err, exitCode, string(realmOutput))
	}

	// Create client
	clientCmd := []string{
		"sh", "-c",
		fmt.Sprintf("/opt/keycloak/bin/kcadm.sh create clients -r %s -s clientId=%s -s name='%s' -s enabled=true -s publicClient=false -s standardFlowEnabled=true -s directAccessGrantsEnabled=true",
			keycloakRealm, keycloakClientID, keycloakClientName),
	}
	exitCode, clientOutputReader, err := kc.Exec(ctx, clientCmd)
	if err != nil || exitCode != 0 {
		clientOutput, _ := ioutil.ReadAll(clientOutputReader)
		return fmt.Errorf("failed to create client: %v, exit code: %d, output: %s", 
			err, exitCode, string(clientOutput))
	}

	// Create realm role - the correct command is "create role" (singular) not "create roles" (plural)
	roleCmd := []string{
		"sh", "-c",
		fmt.Sprintf("/opt/keycloak/bin/kcadm.sh create role -r %s -s name=%s -s 'description=A test role created for integration testing'",
			keycloakRealm, keycloakRoleName),
	}
	exitCode, roleOutputReader, err := kc.Exec(ctx, roleCmd)
	if err != nil || exitCode != 0 {
		roleOutput, _ := ioutil.ReadAll(roleOutputReader)
		return fmt.Errorf("failed to create role: %v, exit code: %d, output: %s", 
			err, exitCode, string(roleOutput))
	}

	return nil
}

// runTerraformerImport runs the Terraformer import command with the given options
func runTerraformerImport(t *testing.T, kc *keycloakContainer, outputDir string, moduleOutput bool) error {
	// Set up environment for Terraformer
	os.Setenv("KEYCLOAK_URL", kc.URI)
	os.Setenv("KEYCLOAK_CLIENT_ID", "admin-cli")
	os.Setenv("KEYCLOAK_USERNAME", kc.Username)
	os.Setenv("KEYCLOAK_PASSWORD", kc.Password)

	// Prepare import options
	importArgs := []string{
		"keycloak",
		"--resources=realm,client,role",
		"--filter=realm=" + keycloakRealm,
		"--path=" + outputDir,
		"--verbose",
	}

	if moduleOutput {
		importArgs = append(importArgs, "--module-output")
	}

	// Run Terraformer import
	options := cmd.ImportOptions{}
	err := cmd.Import(nil, options, importArgs)
	if err != nil {
		return fmt.Errorf("terraformer import failed: %v", err)
	}

	return nil
}

// verifyOutput verifies that the expected output files exist
func verifyOutput(t *testing.T, outputDir string, moduleOutput bool) {
	if moduleOutput {
		// Verify module output
		_, err := os.Stat(filepath.Join(outputDir, "keycloak", "main.tf"))
		assert.NoError(t, err, "main.tf should exist")

		_, err = os.Stat(filepath.Join(outputDir, "keycloak", "variables.tf"))
		assert.NoError(t, err, "variables.tf should exist")

		_, err = os.Stat(filepath.Join(outputDir, "keycloak", "terraform.tfvars.json"))
		assert.NoError(t, err, "terraform.tfvars.json should exist")
	} else {
		// Verify standard output
		_, err := os.Stat(filepath.Join(outputDir, "keycloak", "realm", "terraform.tfstate"))
		assert.NoError(t, err, "realm terraform.tfstate should exist")

		_, err = os.Stat(filepath.Join(outputDir, "keycloak", "client", "terraform.tfstate"))
		assert.NoError(t, err, "client terraform.tfstate should exist")

		_, err = os.Stat(filepath.Join(outputDir, "keycloak", "role", "terraform.tfstate"))
		assert.NoError(t, err, "role terraform.tfstate should exist")
	}
}

// TestKeycloakImport tests importing Keycloak resources with Terraformer
func TestKeycloakImport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	// Setup Keycloak container
	kc, err := setupKeycloak(ctx)
	require.NoError(t, err)
	defer func() {
		if err := kc.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %s", err)
		}
	}()

	// Create test realm and client
	err = createTestRealm(ctx, kc)
	require.NoError(t, err)

	// Create temporary directory for test output
	tempDir, err := os.MkdirTemp("", "terraformer-keycloak-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Run test without module output
	t.Run("WithoutModuleOutput", func(t *testing.T) {
		outputDir := filepath.Join(tempDir, "without-module")
		err = os.MkdirAll(outputDir, 0755)
		require.NoError(t, err)

		err = runTerraformerImport(t, kc, outputDir, false)
		require.NoError(t, err)

		verifyOutput(t, outputDir, false)
	})

	// Run test with module output
	t.Run("WithModuleOutput", func(t *testing.T) {
		outputDir := filepath.Join(tempDir, "with-module")
		err = os.MkdirAll(outputDir, 0755)
		require.NoError(t, err)

		err = runTerraformerImport(t, kc, outputDir, true)
		require.NoError(t, err)

		verifyOutput(t, outputDir, true)
	})
}
