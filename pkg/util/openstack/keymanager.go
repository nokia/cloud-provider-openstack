/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package openstack

import (
	"context"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/keymanager/v1/secrets"
	"k8s.io/cloud-provider-openstack/pkg/metrics"
	cpoerrors "k8s.io/cloud-provider-openstack/pkg/util/errors"
)

// EnsureSecret creates a secret if it doesn't exist.
func EnsureSecret(ctx context.Context, client *gophercloud.ServiceClient, name string, secretType string, payload string) (string, error) {
	secret, err := GetSecret(ctx, client, name)
	if err != nil {
		if err == cpoerrors.ErrNotFound {
			// Create a new one
			return CreateSecret(ctx, client, name, secretType, payload)
		}

		return "", err
	}

	return secret.SecretRef, nil
}

// GetSecret returns the secret by name
func GetSecret(ctx context.Context, client *gophercloud.ServiceClient, name string) (*secrets.Secret, error) {
	listOpts := secrets.ListOpts{
		Name: name,
	}
	mc := metrics.NewMetricContext("secret", "list")
	allPages, err := secrets.List(client, listOpts).AllPages(ctx)
	if mc.ObserveRequest(err) != nil {
		return nil, err
	}
	allSecrets, err := secrets.ExtractSecrets(allPages)
	if err != nil {
		return nil, err
	}

	if len(allSecrets) == 0 {
		return nil, cpoerrors.ErrNotFound
	}
	if len(allSecrets) > 1 {
		return nil, cpoerrors.ErrMultipleResults
	}

	return &allSecrets[0], nil
}

// CreateSecret creates a secret in Barbican, returns the secret url.
func CreateSecret(ctx context.Context, client *gophercloud.ServiceClient, name string, secretType string, payload string) (string, error) {
	createOpts := secrets.CreateOpts{
		Name:                   name,
		Algorithm:              "aes",
		Mode:                   "cbc",
		BitLength:              256,
		PayloadContentType:     secretType,
		PayloadContentEncoding: "base64",
		Payload:                payload,
		SecretType:             secrets.OpaqueSecret,
	}
	mc := metrics.NewMetricContext("secret", "create")
	secret, err := secrets.Create(ctx, client, createOpts).Extract()
	if mc.ObserveRequest(err) != nil {
		return "", err
	}
	return secret.SecretRef, nil
}

// ParseSecretID return secret ID from secretRef
func ParseSecretID(ref string) (string, error) {
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("could not parse %s", ref)
	}

	return parts[len(parts)-1], nil
}

// DeleteSecrets deletes all the secrets that including the name string.
func DeleteSecrets(ctx context.Context, client *gophercloud.ServiceClient, partName string) error {
	listOpts := secrets.ListOpts{
		SecretType: secrets.OpaqueSecret,
	}
	mc := metrics.NewMetricContext("secret", "list")
	allPages, err := secrets.List(client, listOpts).AllPages(ctx)
	if mc.ObserveRequest(err) != nil {
		return err
	}
	allSecrets, err := secrets.ExtractSecrets(allPages)
	if err != nil {
		return err
	}

	for _, s := range allSecrets {
		if strings.Contains(s.Name, partName) {
			secretID, err := ParseSecretID(s.SecretRef)
			if err != nil {
				return err
			}
			mc := metrics.NewMetricContext("secret", "delete")
			err = secrets.Delete(ctx, client, secretID).ExtractErr()
			if mc.ObserveRequest(err) != nil && !cpoerrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}
