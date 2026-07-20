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

package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// BackendTLSConfig builds the TLS config for https-protocol sandbox backends,
// the equivalent of FunctionMaster's serversTransport. By default it skips
// verification (matching the platform's yr-backend-tls@file); set
// BackendTLSVerify to verify, optionally against BackendTLSCAFile. A client
// cert/key pair, if configured, is presented for backend mTLS.
func (c *SandboxRouterConfig) BackendTLSConfig() (*tls.Config, error) {
	cfg := &tls.Config{
		// Platform serversTransport default; opt in via BackendTLSVerify.
		InsecureSkipVerify: !c.BackendTLSVerify, // nolint:gosec
		MinVersion:         tls.VersionTLS12,
		ServerName:         c.BackendTLSServerName,
	}

	if c.BackendTLSVerify && c.BackendTLSCAFile != "" {
		caPEM, err := os.ReadFile(c.BackendTLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read backend CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("no valid certificates in backend CA file %s", c.BackendTLSCAFile)
		}
		cfg.RootCAs = pool
	}

	if c.BackendTLSCertFile != "" || c.BackendTLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.BackendTLSCertFile, c.BackendTLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load backend client cert: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}

	return cfg, nil
}
