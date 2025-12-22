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

// Package etcd3 implements crud and watch operations based etcd clientv3
package etcd3

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"go.etcd.io/etcd/client/v3"

	"frontend/pkg/common/faas_common/crypto"
	"frontend/pkg/common/faas_common/localauth"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/sts"
	"frontend/pkg/common/faas_common/sts/cert"
	commontls "frontend/pkg/common/faas_common/tls"
	"frontend/pkg/common/faas_common/utils"
)

// EtcdAuth etcd authentication interface
type EtcdAuth interface {
	GetEtcdConfig() (*clientv3.Config, error)
}

type noAuth struct {
}

type tlsAuth struct {
	caFile   string
	certFile string
	keyFile  string
	user     string
	password string
}

type pwdAuth struct {
	user     string
	password string
}

type clientTLSAuth struct {
	cerfile        []byte
	keyfile        []byte
	cafile         []byte
	passphrasefile []byte
}

// GetEtcdAuthType etcd authentication type
func GetEtcdAuthType(etcdConfig EtcdConfig) EtcdAuth {
	if etcdConfig.AuthType == "TLS" {
		return &clientTLSAuth{
			cerfile:        []byte(etcdConfig.CertFile),
			keyfile:        []byte(etcdConfig.KeyFile),
			cafile:         []byte(etcdConfig.CaFile),
			passphrasefile: []byte(etcdConfig.PassphraseFile),
		}
	}
	if etcdConfig.SslEnable {
		if os.Getenv(sts.EnvSTSEnable) == "true" {
			return &tlsAuth{}
		}
		return &tlsAuth{
			certFile: etcdConfig.CertFile,
			keyFile:  etcdConfig.KeyFile,
			caFile:   etcdConfig.CaFile,
			user:     etcdConfig.User,
			password: etcdConfig.Password,
		}
	}
	if etcdConfig.Password == "" {
		return &noAuth{}
	}
	if len(etcdConfig.User) != 0 || len(etcdConfig.Password) != 0 {
		return &pwdAuth{
			user:     etcdConfig.User,
			password: etcdConfig.Password,
		}
	}
	return &noAuth{}
}

func (n *noAuth) GetEtcdConfig() (*clientv3.Config, error) {
	return &clientv3.Config{}, nil
}

func (t *tlsAuth) GetEtcdConfig() (*clientv3.Config, error) {
	if os.Getenv(sts.EnvSTSEnable) == "true" {
		return BuildStsCfg()
	}
	pool, err := commontls.GetX509CACertPool(t.caFile)
	if err != nil {
		log.GetLogger().Errorf("failed to getX509CACertPool: %s", err.Error())
		return nil, err
	}

	var certs []tls.Certificate
	if certs, err = commontls.LoadServerTLSCertificate(t.certFile, t.keyFile, "", "LOCAL", false); err != nil {
		log.GetLogger().Errorf("failed to loadServerTLSCertificate: %s", err.Error())
		return nil, err
	}

	clientAuthMode := tls.NoClientCert
	cfg := &clientv3.Config{
		TLS: &tls.Config{
			RootCAs:      pool,
			Certificates: certs,
			ClientAuth:   clientAuthMode,
		},
	}
	if len(t.user) != 0 && len(t.password) != 0 {
		pwd, err := localauth.Decrypt(t.password)
		if err != nil {
			log.GetLogger().Errorf("failed to decrypt etcd config with error %s", err)
			return nil, err
		}
		cfg.Username = t.user
		cfg.Password = string(pwd)
		utils.ClearStringMemory(t.password)
	}
	return cfg, nil
}

func (p *pwdAuth) GetEtcdConfig() (*clientv3.Config, error) {
	if len(p.user) == 0 || len(p.password) == 0 {
		return nil, errors.New("etcd user or password is empty")
	}
	pwd, err := localauth.Decrypt(p.password)
	if err != nil {
		log.GetLogger().Errorf("failed to decrypt etcd config with error %s", err)
		return nil, err
	}
	cfg := &clientv3.Config{
		Username: p.user,
		Password: string(pwd),
	}
	utils.ClearStringMemory(p.password)
	return cfg, nil
}

func (c *clientTLSAuth) getPassphrase() ([]byte, error) {
	// check whether the passphrasefile file exists. If the file exists, the client key is encrypted using a password.
	// If the file does not exist, the client key is not encrypted and can be directly read.
	var keyPwd []byte
	var err error
	if _, err = os.Stat(string(c.passphrasefile)); err == nil {
		keyPwd, err = ioutil.ReadFile(string(c.passphrasefile))
		if err != nil {
			log.GetLogger().Errorf("failed to read passphrasefile, err: %s", err.Error())
			return nil, err
		}
		if crypto.SCCInitialized() {
			pwd, err := crypto.SCCDecrypt(keyPwd)
			if err != nil {
				log.GetLogger().Errorf("failed to decrypt passphrasefile, err: %s", err.Error())
				return nil, err
			}
			keyPwd = []byte(pwd)
		}
	}

	return keyPwd, nil
}

func (c *clientTLSAuth) getTLSConfig(encryptedKeyPEM []byte, keyPwd []byte,
	certPEM []byte, caCertPEM []byte) (*tls.Config, error) {
	// Decode will find the next PEM formatted block (certificate, private key etc) in the input.
	// It returns that block and the remainder of the input.
	// If no PEM data is found, keyBlock is nil and the whole of the input is returned in rest.
	// When keyBlock is nil, an error is reported.
	// You do not need to pay attention to the content of the second return value.
	keyBlock, _ := pem.Decode(encryptedKeyPEM)
	if keyBlock == nil {
		log.GetLogger().Errorf("failed to decode key PEM block")
		return nil, fmt.Errorf("failed to decode key PEM block")
	}

	keyDER := keyBlock.Bytes
	if len(keyDER) == 0 {
	    log.GetLogger().Errorf("keyDER is empty")
	    return nil, fmt.Errorf("keyDER is empty")
	}

	key, err := x509.ParsePKCS1PrivateKey(keyDER)
	if err != nil {
		log.GetLogger().Errorf("failed to parse private key: err: %s", err.Error())
		return nil, err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCertPEM) {
		log.GetLogger().Errorf("failed to append CA certificate")
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	clientCert, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(
		&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	if err != nil {
		log.GetLogger().Errorf("failed to create client certificate: %s", err.Error())
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      certPool,
	}
	return tlsConfig, nil
}

func (c *clientTLSAuth) GetEtcdConfig() (*clientv3.Config, error) {
	keyPwd, err := c.getPassphrase()
	if err != nil {
		return nil, err
	}

	certPEM, err := ioutil.ReadFile(string(c.cerfile))
	if err != nil {
		log.GetLogger().Errorf("failed to read cert file: %s", err.Error())
		return nil, err
	}

	caCertPEM, err := ioutil.ReadFile(string(c.cafile))
	if err != nil {
		log.GetLogger().Errorf("failed to read ca file: %s", err.Error())
		return nil, err
	}

	encryptedKeyPEM, err := ioutil.ReadFile(string(c.keyfile))
	if err != nil {
		log.GetLogger().Errorf("failed to read key file: %s", err.Error())
		return nil, err
	}
	tlsConfig, err := c.getTLSConfig(encryptedKeyPEM, keyPwd, certPEM, caCertPEM)
	if err != nil {
		return nil, err
	}
	return &clientv3.Config{
		TLS: tlsConfig,
	}, nil
}

// BuildStsCfg - Construct tlsConfig from sts p12
func BuildStsCfg() (*clientv3.Config, error) {
	caCertsPool, tlsCert, err := cert.LoadCerts()
	if err != nil {
		log.GetLogger().Errorf("failed to get X509CACertPool and TLSCertificate: %s", err.Error())
		return nil, err
	}

	clientAuthMode := tls.NoClientCert
	tlsConfig := &clientv3.Config{
		TLS: &tls.Config{
			RootCAs:      caCertsPool,
			Certificates: []tls.Certificate{*tlsCert},
			ClientAuth:   clientAuthMode,
		},
	}
	return tlsConfig, nil
}
