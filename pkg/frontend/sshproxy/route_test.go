/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2026. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sshproxy

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"frontend/pkg/common/faas_common/types"
)

const testSSHPort = 2222

func TestParseInstanceRoute(t *testing.T) {
	route, err := parseRoute("yr:instance:instance-1")

	require.NoError(t, err)
	require.Equal(t, "instance-1", route.InstanceID)
	require.Zero(t, route.TargetPort)

	route, err = parseRoute("yr:instance:instance-1:port=2222")
	require.NoError(t, err)
	require.Equal(t, testSSHPort, route.TargetPort)
}

func TestParseRouteRejectsExtraFields(t *testing.T) {
	_, err := parseRoute("yr:instance:instance-1:extra=field")
	require.Error(t, err)
}

func TestParseRouteRejectsLegacyPort(t *testing.T) {
	_, err := parseRoute("yr:instance:instance-1:2222")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid SSH route option")
}

func TestParseRouteRejectsDuplicateOptions(t *testing.T) {
	_, err := parseRoute("yr:instance:instance-1:user=root")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported SSH route option")

	_, err = parseRoute("yr:instance:instance-1:port=22:port=2222")
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate SSH route option port")
}

func TestTunnelHeaderFraming(t *testing.T) {
	header := tunnelHeader{
		TunnelVersion: tunnelVersion,
		InstanceID:    "instance-1",
		Protocol:      "tcp",
		TargetPort:    22,
		RequestID:     "request-1",
	}
	var buffer bytes.Buffer
	require.NoError(t, writeFramedJSON(&buffer, header))

	var decoded tunnelHeader
	require.NoError(t, readFramedJSON(&buffer, &decoded))
	require.Equal(t, header, decoded)
}

func TestStaticKeyAuthorizerRechecksResolvedTarget(t *testing.T) {
	principal := keyPrincipal{subject: "configured-key:test", allowAll: true}
	authorizer := staticKeyAuthorizer{principals: map[string]keyPrincipal{"fingerprint": principal}}
	require.NoError(t, authorizer.authorizeResolvedTarget(principal.subject, route{},
		&types.InstanceSpecification{InstanceID: "instance-1"}))
	require.Error(t, authorizer.authorizeResolvedTarget("unknown", route{},
		&types.InstanceSpecification{InstanceID: "instance-1"}))
}

func TestAllowAllTargetAuthorizerDoesNotRequireSubject(t *testing.T) {
	authorizer := allowAllTargetAuthorizer{}

	require.NoError(t, authorizer.authorizeResolvedTarget("", route{},
		&types.InstanceSpecification{InstanceID: "instance-1"}))
}
