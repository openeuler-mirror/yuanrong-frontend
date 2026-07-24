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

// Package sshproxy provides the frontend SSH bastion and proxy tunnel client.
package sshproxy

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	routePrefix         = "yr"
	routeInstance       = "instance"
	maxUsernameBytes    = 1024
	maxRouteField       = 256
	maxTCPPort          = 65535
	minRouteParts       = 3
	routeMetadataFields = 2
	routeTypeIndex      = 1
	firstRouteField     = 2
)

type route struct {
	InstanceID string
	TargetPort int
}

func parseTargetPort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > maxTCPPort {
		return 0, fmt.Errorf("invalid target port %q", value)
	}
	return port, nil
}

func parseRoute(username string) (route, error) {
	if username == "" || len(username) > maxUsernameBytes {
		return route{}, fmt.Errorf("invalid SSH route username length")
	}
	parts := strings.Split(username, ":")
	if len(parts) < minRouteParts || parts[0] != routePrefix {
		return route{}, fmt.Errorf("invalid SSH route prefix")
	}
	decoded := make([]string, len(parts)-routeMetadataFields)
	for i, value := range parts[firstRouteField:] {
		field, err := url.PathUnescape(value)
		if err != nil || field == "" || len(field) > maxRouteField {
			return route{}, fmt.Errorf("invalid SSH route field %d", i+1)
		}
		decoded[i] = field
	}
	if parts[routeTypeIndex] != routeInstance {
		return route{}, fmt.Errorf("unsupported SSH route type %q", parts[routeTypeIndex])
	}
	if len(decoded) < 1 {
		return route{}, fmt.Errorf("instance SSH route requires instanceID")
	}
	targetPort, err := parseRouteOptions(decoded[1:])
	if err != nil {
		return route{}, err
	}
	return route{InstanceID: decoded[0], TargetPort: targetPort}, nil
}

func parseRouteOptions(fields []string) (int, error) {
	var targetPort int
	for _, field := range fields {
		key, value, found := strings.Cut(field, "=")
		if !found || value == "" {
			return 0, fmt.Errorf("invalid SSH route option %q", field)
		}
		switch key {
		case "port":
			if targetPort != 0 {
				return 0, fmt.Errorf("duplicate SSH route option port")
			}
			port, err := parseTargetPort(value)
			if err != nil {
				return 0, err
			}
			targetPort = port
		default:
			return 0, fmt.Errorf("unsupported SSH route option %q", key)
		}
	}
	return targetPort, nil
}
