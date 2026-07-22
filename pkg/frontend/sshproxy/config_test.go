/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2026. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

package sshproxy

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

const testConfiguredConnectionLimit = 4096

func TestSSHClientAuthDefaultsToEnabled(t *testing.T) {
	t.Setenv(envAuthEnable, "")

	enabled, err := sshClientAuthEnabled()

	require.NoError(t, err)
	require.True(t, enabled)
}

func TestSSHClientAuthCanBeExplicitlyDisabled(t *testing.T) {
	t.Setenv(envAuthEnable, "false")

	enabled, err := sshClientAuthEnabled()

	require.NoError(t, err)
	require.False(t, enabled)
	config := newSSHServerConfig(enabled, nil)
	require.True(t, config.NoClientAuth)
	require.Nil(t, config.PublicKeyCallback)
}

func TestSSHClientAuthRejectsInvalidSetting(t *testing.T) {
	t.Setenv(envAuthEnable, "disabled")

	_, err := sshClientAuthEnabled()

	require.Error(t, err)
}

func TestSSHClientAuthConfiguresPublicKeyCallbackWhenEnabled(t *testing.T) {
	config := newSSHServerConfig(true, map[string]keyPrincipal{})

	require.False(t, config.NoClientAuth)
	require.NotNil(t, config.PublicKeyCallback)
}

func TestSSHConnectionLimitDefaultsTo1024(t *testing.T) {
	t.Setenv(envMaxConnections, "")

	limit, err := parseConnectionLimitEnv()

	require.NoError(t, err)
	require.Equal(t, 1024, limit)
}

func TestSSHConnectionLimitCanBeConfigured(t *testing.T) {
	t.Setenv(envMaxConnections, strconv.Itoa(testConfiguredConnectionLimit))

	limit, err := parseConnectionLimitEnv()

	require.NoError(t, err)
	require.Equal(t, testConfiguredConnectionLimit, limit)
}

func TestSSHConnectionLimitRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"0", "-1", "65536", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(envMaxConnections, value)

			_, err := parseConnectionLimitEnv()

			require.Error(t, err)
		})
	}
}
