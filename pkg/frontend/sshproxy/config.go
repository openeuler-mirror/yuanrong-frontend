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
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"frontend/pkg/common/faas_common/logger/log"
	fronttls "frontend/pkg/common/faas_common/tls"
	frontconfig "frontend/pkg/frontend/config"
)

const (
	envEnable             = "YR_FRONTEND_SSH_ENABLE"
	envAuthEnable         = "YR_FRONTEND_SSH_AUTH_ENABLE"
	envAddress            = "YR_FRONTEND_SSH_ADDR"
	envHostKey            = "YR_FRONTEND_SSH_HOST_KEY"
	envAuthorizedKeys     = "YR_FRONTEND_SSH_AUTHORIZED_KEYS"
	envBackendKey         = "YR_FRONTEND_SSH_BACKEND_KEY"
	envRouteWaitTimeout   = "YR_FRONTEND_SSH_ROUTE_WAIT_TIMEOUT_SECONDS"
	envBackendAttempts    = "YR_FRONTEND_SSH_BACKEND_RETRY_ATTEMPTS"
	envBackendIntervalMS  = "YR_FRONTEND_SSH_BACKEND_RETRY_INTERVAL_MS"
	envMaxConnections     = "YR_FRONTEND_SSH_MAX_CONNECTIONS"
	defaultSSHAddress     = ":2222"
	defaultRouteWait      = 10 * time.Second
	defaultAttempts       = 10
	defaultInterval       = 500 * time.Millisecond
	defaultMaxConnections = 1024
	sshHandshakeTimeout   = 10 * time.Second
	decimalBase           = 10
	parseUintBits         = 32
	connectionLimitBits   = 16
	maxPositiveInt        = 1000
)

type serverConfig struct {
	address         string
	routeWait       time.Duration
	backendAttempts int
	backendInterval time.Duration
	clientAuth      bool
	serverSSH       *ssh.ServerConfig
	backendSigner   ssh.Signer
	backendHostKey  ssh.HostKeyCallback
	tunnelTLSConfig *tls.Config
	authorizer      targetAuthorizer
	connectionLimit int
}

type runtimeSettings struct {
	address         string
	routeWait       time.Duration
	backendAttempts int
	backendInterval time.Duration
	connectionLimit int
}

func loadServerConfig() (*serverConfig, bool, error) {
	enabled, err := sshServerEnabled()
	if err != nil {
		return nil, false, fmt.Errorf("invalid %s: %w", envEnable, err)
	}
	if !enabled {
		return nil, false, nil
	}
	config, err := buildServerConfig()
	if err != nil {
		return nil, false, err
	}
	return config, true, nil
}

func sshServerEnabled() (bool, error) {
	raw := strings.TrimSpace(os.Getenv(envEnable))
	if raw == "" {
		return false, nil
	}
	return strconv.ParseBool(raw)
}

func sshClientAuthEnabled() (bool, error) {
	raw := strings.TrimSpace(os.Getenv(envAuthEnable))
	if raw == "" {
		return true, nil
	}
	return strconv.ParseBool(raw)
}

func loadRuntimeSettings() (runtimeSettings, error) {
	address := valueOrDefault(os.Getenv(envAddress), defaultSSHAddress)
	if _, _, err := net.SplitHostPort(address); err != nil {
		return runtimeSettings{}, fmt.Errorf("invalid %s: %w", envAddress, err)
	}
	wait := defaultRouteWait
	if raw := strings.TrimSpace(os.Getenv(envRouteWaitTimeout)); raw != "" {
		seconds, parseErr := strconv.ParseUint(raw, decimalBase, parseUintBits)
		if parseErr != nil || seconds == 0 {
			return runtimeSettings{}, fmt.Errorf("invalid %s", envRouteWaitTimeout)
		}
		wait = time.Duration(seconds) * time.Second
	}
	backendAttempts, err := parsePositiveIntEnv(envBackendAttempts, defaultAttempts)
	if err != nil {
		return runtimeSettings{}, err
	}
	backendIntervalMS, err := parsePositiveIntEnv(envBackendIntervalMS, int(defaultInterval/time.Millisecond))
	if err != nil {
		return runtimeSettings{}, err
	}
	connectionLimit, err := parseConnectionLimitEnv()
	if err != nil {
		return runtimeSettings{}, err
	}
	return runtimeSettings{
		address:         address,
		routeWait:       wait,
		backendAttempts: backendAttempts,
		backendInterval: time.Duration(backendIntervalMS) * time.Millisecond,
		connectionLimit: connectionLimit,
	}, nil
}

func buildServerConfig() (*serverConfig, error) {
	settings, err := loadRuntimeSettings()
	if err != nil {
		return nil, err
	}
	hostSigner, err := readPrivateKey(envHostKey)
	if err != nil {
		return nil, err
	}
	backendSigner, err := readPrivateKey(envBackendKey)
	if err != nil {
		return nil, err
	}
	clientAuth, err := sshClientAuthEnabled()
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", envAuthEnable, err)
	}
	authorizedKeys := make(map[string]keyPrincipal)
	var authorizer targetAuthorizer = allowAllTargetAuthorizer{}
	if clientAuth {
		authorizedKeysPath := requiredEnv(envAuthorizedKeys)
		if authorizedKeysPath == "" {
			return nil, fmt.Errorf("%s is required when %s is enabled", envAuthorizedKeys, envAuthEnable)
		}
		authorizedKeys, err = readAuthorizedKeys(authorizedKeysPath)
		if err != nil {
			return nil, err
		}
		authorizer = staticKeyAuthorizer{principals: authorizedKeys}
	}
	// Sandbox host keys are instance-specific and instances are dynamic. The
	// frontend authenticates the backend component with its configured SSH key;
	// host-key verification is intentionally disabled for the SSH hop.
	hostKeyCallback := ssh.InsecureIgnoreHostKey()
	tunnelTLSConfig, err := loadTunnelTLSConfig(frontconfig.GetConfig().ComponentMTLSEnable)
	if err != nil {
		return nil, err
	}
	sshConfig := newSSHServerConfig(clientAuth, authorizedKeys)
	sshConfig.AddHostKey(hostSigner)
	return &serverConfig{
		address:         settings.address,
		routeWait:       settings.routeWait,
		backendAttempts: settings.backendAttempts,
		backendInterval: settings.backendInterval,
		clientAuth:      clientAuth,
		serverSSH:       sshConfig,
		backendSigner:   backendSigner,
		backendHostKey:  hostKeyCallback,
		tunnelTLSConfig: tunnelTLSConfig,
		authorizer:      authorizer,
		connectionLimit: settings.connectionLimit,
	}, nil
}

func newSSHServerConfig(clientAuth bool, authorizedKeys map[string]keyPrincipal) *ssh.ServerConfig {
	config := &ssh.ServerConfig{NoClientAuth: !clientAuth}
	if clientAuth {
		config.PublicKeyCallback = func(metadata ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			fingerprint := ssh.FingerprintSHA256(key)
			principal, ok := authorizedKeys[fingerprint]
			if !ok {
				return nil, fmt.Errorf("public key %s is not authorized", fingerprint)
			}
			target, err := parseRoute(metadata.User())
			if err != nil {
				return nil, err
			}
			if err = principal.authorizeRequestedTarget(target); err != nil {
				return nil, err
			}
			return &ssh.Permissions{Extensions: map[string]string{"subject": principal.subject}}, nil
		}
	}
	return config
}

func parsePositiveIntEnv(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 || value > maxPositiveInt {
		return 0, fmt.Errorf("invalid %s", name)
	}
	return value, nil
}

func parseConnectionLimitEnv() (int, error) {
	raw := strings.TrimSpace(os.Getenv(envMaxConnections))
	if raw == "" {
		return defaultMaxConnections, nil
	}
	value, err := strconv.ParseUint(raw, decimalBase, connectionLimitBits)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("invalid %s", envMaxConnections)
	}
	return int(value), nil
}

func readPrivateKey(envName string) (ssh.Signer, error) {
	path := requiredEnv(envName)
	if path == "" {
		return nil, fmt.Errorf("%s is required", envName)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", envName, err)
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", envName, err)
	}
	return signer, nil
}

func readAuthorizedKeys(path string) (map[string]keyPrincipal, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", envAuthorizedKeys, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.GetLogger().Warnf("close %s failed: %s", envAuthorizedKeys, closeErr)
		}
	}()
	keys := make(map[string]keyPrincipal)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, _, _, parseErr := ssh.ParseAuthorizedKey([]byte(line))
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", envAuthorizedKeys, parseErr)
		}
		fingerprint := ssh.FingerprintSHA256(key)
		keys[fingerprint] = keyPrincipal{subject: "configured-key:" + fingerprint, allowAll: true}
	}
	if err = scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", envAuthorizedKeys, err)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%s contains no public keys", envAuthorizedKeys)
	}
	return keys, nil
}

func loadTunnelTLSConfig(enabled bool) (*tls.Config, error) {
	if !enabled {
		return nil, nil
	}
	httpsConfig := frontconfig.GetConfig().HTTPSConfig
	if httpsConfig == nil {
		return nil, fmt.Errorf("frontend HTTPS config is required for component certificate paths")
	}
	return fronttls.NewComponentClientTLSConfig(*httpsConfig)
}

func requiredEnv(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func valueOrDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
