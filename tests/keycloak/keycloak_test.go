package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/terraformer/cmd"
	"github.com/GoogleCloudPlatform/terraformer/providers/keycloak"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// KeycloakTestData represents test resources to create
type KeycloakTestData struct {
	Realms []RealmData
	Users  []UserData
}

type RealmData struct {
	Name        string
	DisplayName string
	Enabled     bool
}

type UserData struct {
	RealmName  string
	Username   string
	Email      string
	FirstName  string
	LastName   string
	Attributes map[string][]string
}

// setupTestData creates test resources in Keycloak
func setupTestData(uri string) error {
	// Get admin token
	token, err := getAdminToken(uri)
	if err != nil {
		return fmt.Errorf("failed to get admin token: %v", err)
	}

	testData := KeycloakTestData{
		Realms: []RealmData{
			{
				Name:        "test-realm-1",
				DisplayName: "Test Realm One",
				Enabled:     true,
			},
			{
				Name:        "test-realm-2", 
				DisplayName: "Test Realm Two",
				Enabled:     true,
			},
		},
		Users: []UserData{
			{
				RealmName: "test-realm-1",
				Username: "test-user-1",
				Email:    "user1@test.com",
				FirstName: "Test",
				LastName: "User",
				Attributes: map[string][]string{
					"testAttr": {"value1"},
				},
			},
			{
				RealmName: "test-realm-2",
				Username: "test-user-2", 
				Email:    "user2@test.com",
				FirstName: "Test",
				LastName: "User",
				Attributes: map[string][]string{
					"testAttr": {"value2"},
				},
			},
		},
	}

	// Create realms
	for _, realm := range testData.Realms {
		if err := createRealm(uri, token, realm); err != nil {
			return fmt.Errorf("failed to create realm %s: %v", realm.Name, err)
		}
	}

	// Create users
	for _, user := range testData.Users {
		if err := createUser(uri, token, user); err != nil {
			return fmt.Errorf("failed to create user %s: %v", user.Username, err)
		}
	}

	return nil
}

func getAdminToken(uri string) (string, error) {
	tokenURL := fmt.Sprintf("%s/realms/master/protocol/openid-connect/token", uri)
	data := strings.NewReader("grant_type=password&client_id=admin-cli&username=admin&password=admin")
	
	resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result["access_token"].(string), nil
}

func createRealm(uri, token string, realm RealmData) error {
	realmURL := fmt.Sprintf("%s/admin/realms", uri)
	payload := map[string]interface{}{
		"realm":       realm.Name,
		"displayName": realm.DisplayName,
		"enabled":     realm.Enabled,
	}

	return makeRequest(http.MethodPost, realmURL, token, payload)
}

func createUser(uri, token string, user UserData) error {
	userURL := fmt.Sprintf("%s/admin/realms/%s/users", uri, user.RealmName)
	payload := map[string]interface{}{
		"username":   user.Username,
		"email":     user.Email,
		"firstName": user.FirstName,
		"lastName":  user.LastName,
		"enabled":   true,
		"attributes": user.Attributes,
	}

	return makeRequest(http.MethodPost, userURL, token, payload)
}

func makeRequest(method, url, token string, payload interface{}) error {
	var body io.Reader
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(payloadBytes))
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// verifyOutput checks the generated files for expected content
func verifyOutput(t *testing.T, outputPath string, moduleOutput bool) {
	if moduleOutput {
		verifyModuleOutput(t, outputPath)
	} else {
		verifyStandardOutput(t, outputPath)
	}
}

func verifyModuleOutput(t *testing.T, outputPath string) {
	modulePath := filepath.Join(outputPath, "module-output")
	
	// Check main.tf
	mainTf, err := os.ReadFile(filepath.Join(modulePath, "main.tf"))
	assert.NoError(t, err)
	mainContent := string(mainTf)
	
	// Verify resource definitions
	assert.Contains(t, mainContent, "resource \"keycloak_realm\"")
	assert.Contains(t, mainContent, "resource \"keycloak_user\"")
	
	// Check variables.tf
	varsTf, err := os.ReadFile(filepath.Join(modulePath, "variables.tf"))
	assert.NoError(t, err)
	varsContent := string(varsTf)
	
	// Verify variable definitions
	assert.Contains(t, varsContent, "variable \"realms\"")
	assert.Contains(t, varsContent, "variable \"users\"")
	
	// Check tfvars files
	tfvarsFiles, err := filepath.Glob(filepath.Join(modulePath, "*.tfvars.json"))
	assert.NoError(t, err)
	
	for _, file := range tfvarsFiles {
		content, err := os.ReadFile(file)
		assert.NoError(t, err)
		
		var tfvars map[string]interface{}
		err = json.Unmarshal(content, &tfvars)
		assert.NoError(t, err)
		
		// Verify test data is present
		if strings.Contains(file, "realm") {
			assert.Contains(t, tfvars, "test-realm-1")
			assert.Contains(t, tfvars, "test-realm-2")
		}
		if strings.Contains(file, "user") {
			assert.Contains(t, tfvars, "test-user-1")
			assert.Contains(t, tfvars, "test-user-2")
		}
	}
}

func verifyStandardOutput(t *testing.T, outputPath string) {
	// Check realm resources
	realmPath := filepath.Join(outputPath, "keycloak", "realms")
	realmTf, err := os.ReadFile(filepath.Join(realmPath, "realms.tf"))
	assert.NoError(t, err)
	realmContent := string(realmTf)
	
	assert.Contains(t, realmContent, "test-realm-1")
	assert.Contains(t, realmContent, "test-realm-2")
	
	// Check user resources
	userPath := filepath.Join(outputPath, "keycloak", "users")
	userTf, err := os.ReadFile(filepath.Join(userPath, "users.tf"))
	assert.NoError(t, err)
	userContent := string(userTf)
	
	assert.Contains(t, userContent, "test-user-1")
	assert.Contains(t, userContent, "test-user-2")
	
	// Verify tfstate files
	assert.FileExists(t, filepath.Join(realmPath, "terraform.tfstate"))
	assert.FileExists(t, filepath.Join(userPath, "terraform.tfstate"))
}

func TestKeycloakImport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	container, err := setupKeycloak(t)
	assert.NoError(t, err)
	defer container.Terminate(context.Background())
	
	// Wait for Keycloak to be fully ready
	time.Sleep(10 * time.Second)
	
	// Setup test data
	err = setupTestData(container.URI)
	assert.NoError(t, err)
	
	testCases := []struct {
		name         string
		moduleOutput bool
		outputPath   string
	}{
		{
			name:         "Standard Output",
			moduleOutput: false,
			outputPath:   "test-output",
		},
		{
			name:         "Module Output",
			moduleOutput: true,
			outputPath:   "test-module-output",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clean output directory before each test
			err := os.RemoveAll(tc.outputPath)
			assert.NoError(t, err)

			options := cmd.ImportOptions{
				Resources:    []string{"realms", "users"},
				PathPattern:  "{output}/{provider}/{service}/",
				PathOutput:   tc.outputPath,
				Connect:      true,
				ModuleOutput: tc.moduleOutput,
			}
			
			args := []string{
				"--url=" + container.URI,
				"--username=admin",
				"--password=admin",
			}
			
			err = cmd.Import(keycloak.NewKeycloakProvider(), options, args)
			assert.NoError(t, err)
			
			// Verify the output
			verifyOutput(t, tc.outputPath, tc.moduleOutput)
		})
	}
}
