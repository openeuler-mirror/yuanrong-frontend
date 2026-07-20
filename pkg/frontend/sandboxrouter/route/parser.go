/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2025. All rights reserved.
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

package route

import (
	"errors"
	"strconv"
	"strings"
)

// ErrInvalidPath means the request path does not resolve to a (safeID, port).
// The proxy maps this to 404, matching Traefik's "no router matched" behaviour.
var ErrInvalidPath = errors.New("invalid sandbox route path")

// ParsePath splits /{safeID}/{port}[/rest] into a Key and the stripped
// path. port must be a sandbox containerPort in [1, 65535]. The leading
// /{safeID}/{port} prefix is removed; the remainder (always starting with '/',
// "/" when empty) is the StrippedPath forwarded upstream.
func ParsePath(path string) (*ParsedRequest, error) {
	if len(path) == 0 || path[0] != '/' {
		return nil, ErrInvalidPath
	}
	// Trim the leading '/', then take the first two segments as id and port.
	rest := path[1:]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return nil, ErrInvalidPath // only one segment, no port
	}
	safeID := rest[:slash]
	rest = rest[slash+1:]

	slash = strings.IndexByte(rest, '/')
	var portStr, tail string
	if slash < 0 {
		portStr, tail = rest, ""
	} else {
		portStr, tail = rest[:slash], rest[slash:]
	}
	if safeID == "" || portStr == "" {
		return nil, ErrInvalidPath
	}

	port, err := strconv.ParseUint(portStr, portBase, portBitSize)
	if err != nil || port == 0 {
		return nil, ErrInvalidPath
	}

	stripped := tail
	if stripped == "" {
		stripped = "/"
	}
	return &ParsedRequest{
		Key:          Key{SafeInstanceID: safeID, Port: uint16(port)},
		StrippedPath: stripped,
	}, nil
}
