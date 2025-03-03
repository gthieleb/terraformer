package keycloak_test

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// KeycloakContainerOptions defines options for creating a Keycloak container
type KeycloakContainerOptions struct {
	Image    string
	Username string
	Password string
	Port     string
	Reuse    bool
}

// NewKeycloakContainer creates a new Keycloak container with default options
func NewKeycloakContainer(ctx context.Context) (*keycloakContainer, error) {
	return NewKeycloakContainerWithOptions(ctx, KeycloakContainerOptions{
		Image:    keycloakImage,
		Username: keycloakUsername,
		Password: keycloakPassword,
		Port:     keycloakPort,
		Reuse:    false,
	})
}

// NewKeycloakContainerWithOptions creates a new Keycloak container with custom options
func NewKeycloakContainerWithOptions(ctx context.Context, options KeycloakContainerOptions) (*keycloakContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        options.Image,
		ExposedPorts: []string{options.Port},
		Env: map[string]string{
			"KEYCLOAK_ADMIN":          options.Username,
			"KEYCLOAK_ADMIN_PASSWORD": options.Password,
			"KC_HEALTH_ENABLED":       "true",
			"KC_METRICS_ENABLED":      "true",
			"KC_FEATURES":             "token-exchange,admin-fine-grained-authz",
			"KC_DB":                   "dev-mem",
		},
		Cmd: []string{"start-dev"},
		WaitingFor: wait.ForAll(
			wait.ForLog("Running the server in development mode"),
			wait.ForHTTP("/health/ready").WithPort(nat.Port(options.Port)).WithStatusCodeMatcher(func(status int) bool {
				return status == 200
			}).WithStartupTimeout(2 * time.Minute),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Reuse:            options.Reuse,
	})
	if err != nil {
		return nil, err
	}

	ip, err := container.Host(ctx)
	if err != nil {
		return nil, err
	}

	mappedPort, err := container.MappedPort(ctx, nat.Port(options.Port))
	if err != nil {
		return nil, err
	}

	uri := fmt.Sprintf("http://%s:%s", ip, mappedPort.Port())

	return &keycloakContainer{
		Container: container,
		URI:       uri,
		Username:  options.Username,
		Password:  options.Password,
	}, nil
}

// CreateRealm creates a new realm in Keycloak
func (k *keycloakContainer) CreateRealm(ctx context.Context, realmName string) error {
	createRealmCmd := []string{
		"/opt/keycloak/bin/kcadm.sh", "create", "realms",
		"--server", k.URI,
		"--user", k.Username,
		"--password", k.Password,
		"-s", fmt.Sprintf("realm=%s", realmName),
		"-s", "enabled=true",
	}

	exitCode, _, err := k.Exec(ctx, createRealmCmd)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to create realm: %v, exit code: %d", err, exitCode)
	}
	return nil
}

// CreateClient creates a new client in a realm
func (k *keycloakContainer) CreateClient(ctx context.Context, realmName, clientID, clientName string) error {
	createClientCmd := []string{
		"/opt/keycloak/bin/kcadm.sh", "create", "clients",
		"--server", k.URI,
		"--user", k.Username,
		"--password", k.Password,
		"-r", realmName,
		"-s", fmt.Sprintf("clientId=%s", clientID),
		"-s", fmt.Sprintf("name=%s", clientName),
		"-s", "enabled=true",
		"-s", "publicClient=false",
		"-s", "standardFlowEnabled=true",
		"-s", "directAccessGrantsEnabled=true",
	}

	exitCode, _, err := k.Exec(ctx, createClientCmd)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to create client: %v, exit code: %d", err, exitCode)
	}
	return nil
}
