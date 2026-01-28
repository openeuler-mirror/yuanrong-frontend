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

package frontendsdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/magiconair/properties"
	"github.com/smartystreets/goconvey/convey"

	"frontend/pkg/common/faas_common/datasystemclient"
	"frontend/pkg/common/faas_common/logger/config"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/sts/raw"
	commonType "frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/faas_common/wisecloudtool"
	"frontend/pkg/common/faas_common/wisecloudtool/serviceaccount"
	wisecloudtypes "frontend/pkg/common/faas_common/wisecloudtool/types"
	frontendConfig "frontend/pkg/frontend/config"
	"frontend/pkg/frontend/frontendsdk/posixsdk"
	"frontend/pkg/frontend/types"
	"frontend/pkg/frontend/watcher"
	"frontend/pkg/frontend/wisecloud"
	"yuanrong.org/kernel/runtime/libruntime/api"
	"yuanrong.org/kernel/runtime/libruntime/common"
	"yuanrong.org/kernel/runtime/libruntime/execution"
)

func TestInit(t *testing.T) {
	convey.Convey("Test frontend init", t, func() {
		frontend := NewFrontend()
		cfg := &types.Config{
			Runtime: types.RuntimeConfig{
				LogConfig: config.CoreInfo{
					Level: "DEBUG",
				},
				SystemAuthConfig: types.SystemAuthConfig{
					Enable:    true,
					AccessKey: "ak",
					SecretKey: "sk",
				},
			},
			RawStsConfig: raw.StsConfig{
				StsEnable: true,
			},
		}
		data, _ := json.Marshal(cfg)
		convey.Convey("config file is empty", func() {
			err := frontend.Init("")
			convey.So(err.Error(), convey.ShouldContainSubstring, "config file path is empty")
		})
		convey.Convey("read config file error", func() {
			err := frontend.Init("/home/sn/config.json")
			convey.So(err.Error(), convey.ShouldContainSubstring, "read config failed")
		})
		convey.Convey("Unmarshal config error", func() {
			defer gomonkey.ApplyFunc(os.ReadFile, func(name string) ([]byte, error) {
				return []byte{}, nil
			}).Reset()
			err := frontend.Init("/home/sn/config.json")
			convey.So(err.Error(), convey.ShouldContainSubstring, "unmarshal config failed")
		})
		convey.Convey("sts init failed", func() {
			defer gomonkey.ApplyFunc(os.ReadFile, func(name string) ([]byte, error) {
				return data, nil
			}).Reset()
			err := frontend.Init("/home/sn/config.json")
			convey.So(err.Error(), convey.ShouldContainSubstring, "failed to init sts sdk")
		})
		convey.Convey("init logger error", func() {
			defer gomonkey.ApplyFunc(os.ReadFile, func(name string) ([]byte, error) {
				return data, nil
			}).Reset()
			defer gomonkey.ApplyFunc(stsgoapi.InitWith, func(property properties.Properties) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(stsgoapi.DecryptSensitiveConfig,
				func(rawConfigValue string) (plainBytes []byte, err error) {
					return plainBytes, nil
				}).Reset()
			defer gomonkey.ApplyFunc(log.InitRunLog, func(fileName string, isAsync bool) error {
				return errors.New("init log error")
			}).Reset()
			defer gomonkey.ApplyFunc(parseServiceAccountJwt, func(config2 *types.Config) error {
				return nil
			}).Reset()
			err := frontend.Init("/home/sn/config.json")
			convey.So(err.Error(), convey.ShouldContainSubstring, "init logger error")
		})
		convey.Convey("init runtime failed", func() {
			defer gomonkey.ApplyFunc(os.ReadFile, func(name string) ([]byte, error) {
				return data, nil
			}).Reset()
			defer gomonkey.ApplyFunc(stsgoapi.InitWith, func(property properties.Properties) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(stsgoapi.DecryptSensitiveConfig,
				func(rawConfigValue string) (plainBytes []byte, err error) {
					return plainBytes, nil
				}).Reset()
			defer gomonkey.ApplyFunc(log.InitRunLog, func(fileName string, isAsync bool) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(posixsdk.InitRuntime, func(conf *common.Configuration,
				intfs execution.FunctionExecutionIntfs) error {
				return errors.New("init InitRuntime error")
			}).Reset()
			defer gomonkey.ApplyFunc(parseServiceAccountJwt, func(config2 *types.Config) error {
				return nil
			}).Reset()
			err := frontend.Init("/home/sn/config.json")
			convey.So(err.Error(), convey.ShouldContainSubstring, "init InitRuntime error")
		})
		convey.Convey("init runtime success", func() {
			defer gomonkey.ApplyFunc(os.ReadFile, func(name string) ([]byte, error) {
				return data, nil
			}).Reset()
			defer gomonkey.ApplyFunc(stsgoapi.InitWith, func(property properties.Properties) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(stsgoapi.DecryptSensitiveConfig,
				func(rawConfigValue string) (plainBytes []byte, err error) {
					return plainBytes, nil
				}).Reset()
			defer gomonkey.ApplyFunc(log.InitRunLog, func(fileName string, isAsync bool) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(posixsdk.InitRuntime, func(conf *common.Configuration,
				intfs execution.FunctionExecutionIntfs) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(parseServiceAccountJwt, func(config2 *types.Config) error {
				return nil
			}).Reset()
			err := frontend.Init("/home/sn/config.json")
			convey.So(err, convey.ShouldBeNil)
		})
	})
}

func TestInitSDKHandler(t *testing.T) {
	convey.Convey("Test initSDKHandler", t, func() {
		args := []api.Arg{{
			Data: []byte("hello"),
		}}
		convey.Convey("init frontend config fail", func() {
			defer gomonkey.ApplyFunc(frontendConfig.InitFunctionConfig, func(data []byte) error {
				return errors.New("failed to parse the config data")
			}).Reset()
			_, err := initSDKHandler(args, nil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "failed to parse the config data")
		})

		convey.Convey("failed to init etcd", func() {
			defer gomonkey.ApplyFunc(frontendConfig.InitFunctionConfig, func(data []byte) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(frontendConfig.InitEtcd, func(stopCh <-chan struct{}) error {
				return errors.New("failed to init etcd")
			}).Reset()
			_, err := initSDKHandler(args, nil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "failed to init etcd")
		})

		convey.Convey("failed to watch etcd", func() {
			defer gomonkey.ApplyFunc(frontendConfig.InitFunctionConfig, func(data []byte) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(frontendConfig.InitEtcd, func(stopCh <-chan struct{}) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(watcher.StartWatch, func(stopCh <-chan struct{}) error {
				return errors.New("failed to watch etcd")
			}).Reset()
			_, err := initSDKHandler(args, nil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "failed to watch etcd")
		})

		convey.Convey("success", func() {
			defer gomonkey.ApplyFunc(frontendConfig.InitFunctionConfig, func(data []byte) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(frontendConfig.InitEtcd, func(stopCh <-chan struct{}) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(watcher.StartWatch, func(stopCh <-chan struct{}) error {
				return nil
			}).Reset()
			defer gomonkey.ApplyFunc(datasystemclient.InitDataSystemLibruntime, func(cfg *commonType.DataSystemConfig,
				rt api.LibruntimeAPI, stopCh <-chan struct{}) {
			}).Reset()
			defer gomonkey.ApplyFunc(wisecloud.NewColdStartProvider, func(serviceAccountJwt *wisecloudtypes.ServiceAccountJwt) *wisecloudtool.PodOperator {
				return nil
			}).Reset()
			_, err := initSDKHandler(args, nil)
			convey.So(err, convey.ShouldBeNil)
		})
	})
}

func TestParseSystemAuthError(t *testing.T) {
	convey.Convey("Test ParseSystemAuth Error", t, func() {
		cfg := &types.Config{
			Runtime: types.RuntimeConfig{
				SystemAuthConfig: types.SystemAuthConfig{
					Enable:    false,
					AccessKey: "ak",
					SecretKey: "sk",
				},
			},
		}
		convey.Convey("systemAuth not enable ", func() {
			err := parseSystemAuth(cfg, &common.Configuration{})
			convey.So(err, convey.ShouldBeNil)
		})
		convey.Convey("DecryptSensitiveConfig ak error ", func() {
			defer gomonkey.ApplyFunc(stsgoapi.DecryptSensitiveConfig,
				func(rawConfigValue string) (plainBytes []byte, err error) {
					if rawConfigValue == "ak" {
						return []byte{}, errors.New("decrypt accessKey failed")
					}
					return plainBytes, nil
				}).Reset()
			cfg.Runtime.SystemAuthConfig.Enable = true
			err := parseSystemAuth(cfg, &common.Configuration{})
			convey.So(err.Error(), convey.ShouldContainSubstring, "decrypt accessKey failed")
		})
		convey.Convey("DecryptSensitiveConfig sk error ", func() {
			defer gomonkey.ApplyFunc(stsgoapi.DecryptSensitiveConfig,
				func(rawConfigValue string) (plainBytes []byte, err error) {
					if rawConfigValue == "sk" {
						return []byte{}, errors.New("decrypt secretKey failed")
					}
					return plainBytes, nil
				}).Reset()
			cfg.Runtime.SystemAuthConfig.Enable = true
			err := parseSystemAuth(cfg, &common.Configuration{})
			convey.So(err.Error(), convey.ShouldContainSubstring, "decrypt secretKey failed")
		})
	})
}

func TestSetEnvError(t *testing.T) {
	convey.Convey("Test SetEnv Error", t, func() {
		cfg := &types.Config{
			Runtime: types.RuntimeConfig{
				LogConfig: config.CoreInfo{
					Level: "INFO",
				},
			},
		}
		configFilePath := "/home/sn/config"
		convey.Convey("json Marshal error ", func() {
			defer gomonkey.ApplyFunc(json.Marshal, func(v interface{}) ([]byte, error) {
				return nil, errors.New("json marshal error")
			}).Reset()
			err := setEnv(configFilePath, cfg)
			convey.So(err.Error(), convey.ShouldContainSubstring, "json marshal error")
		})
		convey.Convey("set logConfigKey error ", func() {
			defer gomonkey.ApplyFunc(os.Setenv, func(key, value string) error {
				if key == logConfigKey {
					return errors.New("set env logConfigKey error")
				}
				return nil
			}).Reset()
			err := setEnv(configFilePath, cfg)
			convey.So(err.Error(), convey.ShouldContainSubstring, "set env logConfigKey error")
		})
		convey.Convey("set initArgsFilePathEnvKey error ", func() {
			defer gomonkey.ApplyFunc(os.Setenv, func(key, value string) error {
				if key == initArgsFilePathEnvKey {
					return errors.New("set env initArgsFilePathEnvKey error")
				}
				return nil
			}).Reset()
			err := setEnv(configFilePath, cfg)
			convey.So(err.Error(), convey.ShouldContainSubstring, "set env initArgsFilePathEnvKey error")
		})
		convey.Convey("set dataSystemAddr error ", func() {
			defer gomonkey.ApplyFunc(os.Setenv, func(key, value string) error {
				if key == dataSystemAddr {
					return errors.New("set env dataSystemAddr error")
				}
				return nil
			}).Reset()
			err := setEnv(configFilePath, cfg)
			convey.So(err.Error(), convey.ShouldContainSubstring, "set env dataSystemAddr error")
		})
	})
}

func TestCheckLocalDataSystemStatusReady(t *testing.T) {
	convey.Convey("test check local dataSystem status ready", t, func() {
		frontend := NewFrontend()
		convey.Convey("test check local dataSystem status, when streamEnable is false", func() {
			defer gomonkey.ApplyFunc(frontendConfig.GetConfig, func() *types.Config {
				return &types.Config{
					StreamEnable: false,
				}
			}).Reset()
			result := frontend.CheckLocalDataSystemStatusReady()
			convey.So(result, convey.ShouldBeTrue)
		})

		convey.Convey("test check local dataSystem status, when it's not local dataSystem or not ready", func() {
			defer gomonkey.ApplyFunc(frontendConfig.GetConfig, func() *types.Config {
				return &types.Config{
					StreamEnable: true,
				}
			}).Reset()
			defer gomonkey.ApplyFunc(datasystemclient.IsLocalDataSystemStatusReady, func() bool {
				return false
			}).Reset()
			result := frontend.CheckLocalDataSystemStatusReady()
			convey.So(result, convey.ShouldBeFalse)
		})

		convey.Convey("test check local dataSystem status, when it's local dataSystem and ready", func() {
			defer gomonkey.ApplyFunc(frontendConfig.GetConfig, func() *types.Config {
				return &types.Config{
					StreamEnable: true,
				}
			}).Reset()
			defer gomonkey.ApplyFunc(datasystemclient.IsLocalDataSystemStatusReady, func() bool {
				return true
			}).Reset()
			result := frontend.CheckLocalDataSystemStatusReady()
			convey.So(result, convey.ShouldBeTrue)
		})
	})
}

func TestParseServiceAccountJwt(t *testing.T) {
	convey.Convey("Given a Config instance", t, func() {

		convey.Convey("When STS is enabled and ServiceAccountKeyStr is not empty", func() {
			cfg := &types.Config{
				RawStsConfig: raw.StsConfig{StsEnable: true},
				WiseCloudConfig: types.WiseCloudConfig{
					ServiceAccountJwt: wisecloudtypes.ServiceAccountJwt{
						ServiceAccountKeyStr: "service-account-key",
					},
				},
			}

			parsedServiceAccount := &wisecloudtypes.ServiceAccount{}
			defer gomonkey.ApplyFunc(serviceaccount.ParseServiceAccount, func(keyStr string) (*wisecloudtypes.ServiceAccount, error) {
				return parsedServiceAccount, nil
			}).Reset()

			err := parseServiceAccountJwt(cfg)

			convey.Convey("Then ParseServiceAccount should be called", func() {
				convey.So(err, convey.ShouldBeNil)
				convey.So(cfg.WiseCloudConfig.ServiceAccountJwt.ServiceAccount, convey.ShouldEqual, parsedServiceAccount)
				convey.So(frontendConfig.GetConfig().WiseCloudConfig.ServiceAccountJwt.ServiceAccount, convey.ShouldEqual, parsedServiceAccount)
			})
		})

		convey.Convey("When STS is disabled or ServiceAccountKeyStr is empty", func() {
			cfg := &types.Config{
				RawStsConfig: raw.StsConfig{StsEnable: false},
				WiseCloudConfig: types.WiseCloudConfig{
					ServiceAccountJwt: wisecloudtypes.ServiceAccountJwt{
						ServiceAccountKeyStr: "",
					},
				},
			}

			err := parseServiceAccountJwt(cfg)

			convey.Convey("Then the function should return nil", func() {
				convey.So(err, convey.ShouldBeNil)
			})
		})

		convey.Convey("When TLS config is not nil and TlsCipherSuitesStr is not empty", func() {
			cfg := &types.Config{
				RawStsConfig: raw.StsConfig{StsEnable: true},
				WiseCloudConfig: types.WiseCloudConfig{
					ServiceAccountJwt: wisecloudtypes.ServiceAccountJwt{
						ServiceAccountKeyStr: "service-account-key",
						TlsConfig: &wisecloudtypes.TLSConfig{
							TlsCipherSuitesStr: []string{"cipher-suites"},
						},
					},
				},
			}

			parsedServiceAccount := &wisecloudtypes.ServiceAccount{}
			defer gomonkey.ApplyFunc(serviceaccount.ParseServiceAccount, func(keyStr string) (*wisecloudtypes.ServiceAccount, error) {
				return parsedServiceAccount, nil
			}).Reset()

			parsedTlsCipherSuites := []uint16{1, 2}
			defer gomonkey.ApplyFunc(serviceaccount.ParseTlsCipherSuites, func(cipherSuitesStr []string) ([]uint16, error) {
				return parsedTlsCipherSuites, nil
			}).Reset()

			err := parseServiceAccountJwt(cfg)

			convey.Convey("Then ParseTlsCipherSuites should be called", func() {
				convey.So(err, convey.ShouldBeNil)
				convey.So(cfg.WiseCloudConfig.ServiceAccountJwt.TlsConfig.TlsCipherSuites, convey.ShouldResemble, parsedTlsCipherSuites)
				convey.So(frontendConfig.GetConfig().WiseCloudConfig.ServiceAccountJwt.TlsConfig.TlsCipherSuites, convey.ShouldResemble, parsedTlsCipherSuites)
			})
		})

		convey.Convey("When TLS config is nil or TlsCipherSuitesStr is empty", func() {
			cfg := &types.Config{
				RawStsConfig: raw.StsConfig{StsEnable: true},
				WiseCloudConfig: types.WiseCloudConfig{
					ServiceAccountJwt: wisecloudtypes.ServiceAccountJwt{
						ServiceAccountKeyStr: "service-account-key",
						TlsConfig:            nil,
					},
				},
			}

			err := parseServiceAccountJwt(cfg)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "decrypt service account key failed")

		})

		convey.Convey("When parsing ServiceAccount fails", func() {
			cfg := &types.Config{
				RawStsConfig: raw.StsConfig{StsEnable: true},
				WiseCloudConfig: types.WiseCloudConfig{
					ServiceAccountJwt: wisecloudtypes.ServiceAccountJwt{
						ServiceAccountKeyStr: "invalid-service-account-key",
					},
				},
			}

			defer gomonkey.ApplyFunc(serviceaccount.ParseServiceAccount, func(keyStr string) (*wisecloudtypes.ServiceAccount, error) {
				return nil, fmt.Errorf("failed to parse service account")
			}).Reset()

			err := parseServiceAccountJwt(cfg)

			convey.Convey("Then the function should return the error", func() {
				convey.So(err, convey.ShouldNotBeNil)
				convey.So(err.Error(), convey.ShouldEqual, "failed to parse service account")
			})
		})

		convey.Convey("When parsing TlsCipherSuites fails", func() {
			cfg := &types.Config{
				RawStsConfig: raw.StsConfig{StsEnable: true},
				WiseCloudConfig: types.WiseCloudConfig{
					ServiceAccountJwt: wisecloudtypes.ServiceAccountJwt{
						ServiceAccountKeyStr: "service-account-key",
						TlsConfig: &wisecloudtypes.TLSConfig{
							TlsCipherSuitesStr: []string{"invalid-cipher-suites"},
						},
					},
				},
			}

			parsedServiceAccount := &wisecloudtypes.ServiceAccount{}
			defer gomonkey.ApplyFunc(serviceaccount.ParseServiceAccount, func(keyStr string) (*wisecloudtypes.ServiceAccount, error) {
				return parsedServiceAccount, nil
			}).Reset()
			defer gomonkey.ApplyFunc(serviceaccount.ParseTlsCipherSuites, func(cipherSuitesStr []string) ([]uint16, error) {
				return nil, fmt.Errorf("failed to parse cipher suites")
			}).Reset()

			err := parseServiceAccountJwt(cfg)
			convey.Convey("Then the function should return the error", func() {
				convey.So(err, convey.ShouldNotBeNil)
				convey.So(err.Error(), convey.ShouldEqual, "failed to parse cipher suites")
			})
		})
	})
}
