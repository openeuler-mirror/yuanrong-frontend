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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const certFileMode = 0o600

// writeSelfSigned writes a self-signed cert and its key to temp files, usable
// both as a CA and as a client cert/key pair.
func writeSelfSigned(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "sandboxrouter-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certFile, certPEM, certFileMode); err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyFile, keyPEM, certFileMode); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

// Default (BackendTLSVerify=false) mirrors the platform serversTransport: skip verify.
func TestBackendTLSConfigDefaultSkipsVerify(t *testing.T) {
	c := &SandboxRouterConfig{}
	tlsCfg, err := c.BackendTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Error("default BackendTLSConfig should skip verify (platform-aligned)")
	}
}

func TestBackendTLSConfigVerifyEnabled(t *testing.T) {
	c := &SandboxRouterConfig{BackendTLSVerify: true, BackendTLSServerName: "sb.local"}
	tlsCfg, err := c.BackendTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg.InsecureSkipVerify {
		t.Error("BackendTLSVerify=true should verify")
	}
	if tlsCfg.ServerName != "sb.local" {
		t.Errorf("ServerName = %q, want sb.local", tlsCfg.ServerName)
	}
}

func TestBackendTLSConfigWithCA(t *testing.T) {
	caFile, _ := writeSelfSigned(t)
	c := &SandboxRouterConfig{BackendTLSVerify: true, BackendTLSCAFile: caFile}
	tlsCfg, err := c.BackendTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg.RootCAs == nil {
		t.Error("RootCAs should be set when a valid CA file is given")
	}
}

func TestBackendTLSConfigBadCAPath(t *testing.T) {
	c := &SandboxRouterConfig{BackendTLSVerify: true, BackendTLSCAFile: "/no/such/ca.pem"}
	if _, err := c.BackendTLSConfig(); err == nil {
		t.Error("expected error for unreadable CA file")
	}
}

func TestBackendTLSConfigWithClientCert(t *testing.T) {
	certFile, keyFile := writeSelfSigned(t)
	c := &SandboxRouterConfig{BackendTLSCertFile: certFile, BackendTLSKeyFile: keyFile}
	tlsCfg, err := c.BackendTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("Certificates = %d, want 1", len(tlsCfg.Certificates))
	}
}

func TestBackendTLSConfigBadClientCert(t *testing.T) {
	c := &SandboxRouterConfig{BackendTLSCertFile: "/no/cert.pem", BackendTLSKeyFile: "/no/key.pem"}
	if _, err := c.BackendTLSConfig(); err == nil {
		t.Error("expected error for unreadable client cert")
	}
}
