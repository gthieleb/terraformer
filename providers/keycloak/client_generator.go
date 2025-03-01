// Copyright 2018 The Terraformer Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package keycloak

import (
	"github.com/GoogleCloudPlatform/terraformer/terraformutils"
)

type ClientGenerator struct {
	KeycloakService
}

func (g *ClientGenerator) InitResources() error {
	client, err := g.getKeycloakClient()
	if err != nil {
		return err
	}

	realm := g.Args["realm"].(string)
	clients, err := client.GetClients(realm)
	if err != nil {
		return err
	}

	for _, client := range clients {
		resource := terraformutils.NewResource(
			client.ID,
			client.ClientID,
			"keycloak_openid_client",
			"keycloak",
			map[string]string{
				"realm_id": realm,
				"client_id": client.ClientID,
			},
			[]string{},
			map[string]interface{}{},
		)
		g.Resources = append(g.Resources, resource)
	}

	return nil
}
