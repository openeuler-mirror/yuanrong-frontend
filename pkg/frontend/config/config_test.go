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

// Package config is used to keep the config used by the faas frontend function
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/asaskevich/govalidator/v11"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"

	"frontend/pkg/common/faas_common/alarm"
	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/sts"
	"frontend/pkg/common/faas_common/sts/raw"
	"frontend/pkg/common/faas_common/utils"
	mockUtils "frontend/pkg/common/faas_common/utils"
	"frontend/pkg/frontend/types"
)

func Test_InitFunctionConfig(t *testing.T) {
	convey.Convey("init config error 1", t, func() {
		cfg := &types.Config{}
		inputCfg, _ := json.Marshal(cfg)
		defer gomonkey.ApplyFunc(json.Unmarshal, func(data []byte, v any) error {
			return fmt.Errorf("unmarshal error")
		}).Reset()
		err := InitFunctionConfig(inputCfg)
		convey.So(err, convey.ShouldNotBeNil)
	})

	convey.Convey("init config error 2", t, func() {
		defer gomonkey.ApplyFunc(govalidator.ValidateStruct, func(s interface{}) (bool, error) {
			return true, nil
		}).Reset()
		cfg := &types.Config{}
		cfg.BusinessType = constant.BusinessTypeWiseCloud
		cfg.DataSystemConfig = nil
		inputCfg, _ := json.Marshal(cfg)
		err := InitFunctionConfig(inputCfg)
		convey.So(err, convey.ShouldNotBeNil)
	})

	convey.Convey("init config error 3", t, func() {
		defer gomonkey.ApplyFunc(govalidator.ValidateStruct, func(s interface{}) (bool, error) {
			return true, nil
		}).Reset()
		defer gomonkey.ApplyFunc(setAlarmEnv, func(FConfig *types.Config) error {
			return fmt.Errorf("setAlarmEnv error ")
		}).Reset()
		cfg := &types.Config{}
		inputCfg, _ := json.Marshal(cfg)
		err := InitFunctionConfig(inputCfg)
		convey.So(err, convey.ShouldNotBeNil)
	})

	convey.Convey("init config success", t, func() {
		cfg := &types.Config{}
		inputCfg, _ := json.Marshal(cfg)
		err := InitFunctionConfig(inputCfg)
		convey.So(err, convey.ShouldNotBeNil)
	})
	convey.Convey("init default maxStreamRequestBodySize", t, func() {
		defer gomonkey.ApplyFunc(govalidator.ValidateStruct, func(s interface{}) (bool, error) {
			return true, nil
		}).Reset()
		cfg := &types.Config{}
		inputCfg, _ := json.Marshal(cfg)
		err := InitFunctionConfig(inputCfg)
		convey.So(err, convey.ShouldBeNil)
		convey.ShouldEqual(GetConfig().HTTPConfig.MaxStreamRequestBodySize, 1024)
	})
	convey.Convey("init config success", t, func() {
		defer gomonkey.ApplyFunc(govalidator.ValidateStruct, func(s interface{}) (bool, error) {
			return true, nil
		}).Reset()
		cfg := &types.Config{}
		inputCfg, _ := json.Marshal(cfg)
		err := InitFunctionConfig(inputCfg)
		convey.So(err, convey.ShouldBeNil)
	})
}

func Test_InitModuleConfig(t *testing.T) {
	convey.Convey("init from env failed", t, func() {
		err := InitModuleConfig()
		convey.So(err, convey.ShouldNotBeNil)
	})
	convey.Convey("init from env json error", t, func() {
		defer gomonkey.ApplyFunc(os.Getenv, func(key string) string {
			return `{"http":{"maxRequestBodySize":5}, "cpu":500, "memory":500}`
		}).Reset()
		err := InitModuleConfig()
		convey.So(err, convey.ShouldNotBeNil)
	})
	convey.Convey("init from env success", t, func() {
		defer gomonkey.ApplyFunc(os.Getenv, func(key string) string {
			return `{"http":{"maxRequestBodySize":5}, "cpu":500, "memory":500, "metaEtcd":{"servers":[]}, "routerEtcd":{"servers":[]}, "slaQuota":1000, "runtime":{}}`
		}).Reset()
		err := InitModuleConfig()
		convey.So(err, convey.ShouldBeNil)
	})
	convey.Convey("init default maxStreamRequestBodySize", t, func() {
		defer gomonkey.ApplyFunc(os.Getenv, func(key string) string {
			return `{"http":{"maxRequestBodySize":5}, "cpu":500, "memory":500, "metaEtcd":{"servers":[]}, "routerEtcd":{"servers":[]}, "slaQuota":1000, "runtime":{}}`
		}).Reset()
		err := InitModuleConfig()
		convey.So(err, convey.ShouldBeNil)
		convey.ShouldEqual(GetConfig().HTTPConfig.MaxStreamRequestBodySize, 1024)
	})
}

func TestRecoverConfig(t *testing.T) {
	cfgByte := []byte(`{"Config":{
			"slaQuota": 1000,
			"functionCapability": 1,
			"authenticationEnable": false,
			"trafficLimitDisable": true,
			"http": {
                "resptimeout": 5,
                "workerInstanceReadTimeOut": 5,
                "maxRequestBodySize": 6
            },
		"routerEtcd": {
			"servers": ["1.2.3.4:1234"],
			"user": "tom",
			"password": "**"
		},
		"metaEtcd": {
			"servers": ["1.2.3.4:5678"],
			"user": "tom",
			"password": "**"
		}
		}}`)
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "TestRecoverConfig",
			wantErr: false,
		},
	}
	cfg := types.Config{}
	json.Unmarshal(cfgByte, &cfg)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := RecoverConfig(cfg); (err != nil) != tt.wantErr {
				t.Errorf("RecoverConfig() error = %v, wantErr %v", err, tt.wantErr)
				cfgTest := GetConfig()
				assert.Equal(t, cfgTest.SLAQuota, 1000)
				assert.Equal(t, cfgTest.RouterEtcd.Servers, []string{"1.2.3.4:1234"})
			}
		})
	}

	convey.Convey("RecoverConfig error", t, func() {
		defer gomonkey.ApplyFunc(utils.DeepCopyObj, func(src interface{}, dst interface{}) error {
			return fmt.Errorf("DeepCopyObj error")
		}).Reset()
		err := RecoverConfig(cfg)
		convey.So(err, convey.ShouldNotBeNil)
	})
}

func TestInitConfig(t *testing.T) {
	cfg := []byte(`{
			"slaQuota": 1000,
			"functionCapability": 1,
			"authenticationEnable": false,
			"trafficLimitDisable": true,
			"rawStsConfig": {"stsEnable": true},
			"http": {
                "resptimeout": 5,
                "workerInstanceReadTimeOut": 5,
                "maxRequestBodySize": 6
            },
		"routerEtcd": {
			"servers": ["1.2.3.4:1234"],
			"user": "tom",
			"password": "**"
		},
		"metaEtcd": {
			"servers": ["1.2.3.4:5678"],
			"user": "tom",
			"password": "**"
		}
		}`)
	type args struct {
		data []byte
	}
	tests := []struct {
		name        string
		args        args
		wantErr     assert.ErrorAssertionFunc
		patchesFunc mockUtils.PatchesFunc
	}{
		{"case1 succeed to init config when caas", args{data: cfg}, assert.NoError, func() mockUtils.PatchSlice {
			patches := mockUtils.InitPatchSlice()
			patches.Append(mockUtils.PatchSlice{
				gomonkey.ApplyFunc(govalidator.ValidateStruct, func(s interface{}) (bool, error) { return true, nil }),
				gomonkey.ApplyFunc(sts.InitStsSDK, func(serverCfg raw.ServerConfig) error { return nil }),
			})
			return patches
		}},
		{"case2 failed to init config when caas", args{data: cfg}, assert.Error, func() mockUtils.PatchSlice {
			patches := mockUtils.InitPatchSlice()
			patches.Append(mockUtils.PatchSlice{
				gomonkey.ApplyFunc(govalidator.ValidateStruct, func(s interface{}) (bool, error) { return true, nil }),
				gomonkey.ApplyFunc(sts.InitStsSDK, func(serverCfg raw.ServerConfig) error { return errors.New("e") }),
			})
			return patches
		}},
		{"case3 failed to init config when caas 2", args{data: cfg}, assert.Error, func() mockUtils.PatchSlice {
			patches := mockUtils.InitPatchSlice()
			patches.Append(mockUtils.PatchSlice{
				gomonkey.ApplyFunc(govalidator.ValidateStruct, func(s interface{}) (bool, error) { return true, nil }),
				gomonkey.ApplyFunc(sts.InitStsSDK, func(serverCfg raw.ServerConfig) error { return nil }),
				gomonkey.ApplyFunc(os.Setenv, func(key, value string) error { return errors.New("e") }),
			})
			return patches
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := tt.patchesFunc()
			tt.wantErr(t, InitFunctionConfig(tt.args.data), fmt.Sprintf("InitFunctionConfig(%v)", tt.args.data))
			patches.ResetAll()
		})
	}
}

func TestInitEtcd(t *testing.T) {
	type args struct {
		stopCh <-chan struct{}
	}
	tests := []struct {
		name        string
		args        args
		wantErr     assert.ErrorAssertionFunc
		patchesFunc mockUtils.PatchesFunc
	}{
		{"case1 succeed to init etcd", args{stopCh: make(<-chan struct{})}, assert.NoError, func() mockUtils.PatchSlice {
			patches := mockUtils.InitPatchSlice()
			patches.Append(mockUtils.PatchSlice{
				gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdInitParam{}), "InitClient", func(_ *etcd3.EtcdInitParam) error { return nil }),
			})
			return patches
		}},
		{"case2 failed to init etcd", args{stopCh: make(<-chan struct{})}, assert.Error, func() mockUtils.PatchSlice {
			patches := mockUtils.InitPatchSlice()
			patches.Append(mockUtils.PatchSlice{
				gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdInitParam{}), "InitClient", func(_ *etcd3.EtcdInitParam) error { return errors.New("e") }),
			})
			return patches
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := tt.patchesFunc()
			tt.wantErr(t, InitEtcd(tt.args.stopCh), fmt.Sprintf("InitEtcd(%v)", tt.args.stopCh))
			patches.ResetAll()
		})
	}
}

func TestClearSensitiveInfo(t *testing.T) {
	cfg := []byte(`{
			"slaQuota": 1000,
			"functionCapability": 1,
			"authenticationEnable": false,
			"trafficLimitDisable": true,
			"rawStsConfig": {"stsEnable": true},
			"smsConfig": {"accessKey": "ak"},
			"http": {
                "resptimeout": 5,
                "workerInstanceReadTimeOut": 5,
                "maxRequestBodySize": 6
            },
		"routerEtcd": {
			"servers": ["1.2.3.4:1234"],
			"user": "tom",
			"password": "**"
		},
		"metaEtcd": {
			"servers": ["1.2.3.4:5678"],
			"user": "tom",
			"password": "**"
		}
		}`)
	defer gomonkey.ApplyFunc(govalidator.ValidateStruct, func(s interface{}) (bool, error) {
		return true, nil
	}).Reset()
	_ = InitFunctionConfig(cfg)
	tests := []struct {
		name string
	}{
		{"case1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearSensitiveInfo()
		})
	}
}

func TestSetAlarmEnv(t *testing.T) {
	err := setAlarmEnv(&types.Config{AlarmConfig: alarm.Config{
		EnableAlarm: true,
	}})
	assert.Nil(t, err)
}

func TestInitWatchConfig(t *testing.T) {
	convey.Convey("Test initWatchConfig", t, func() {
		convey.Convey("when the file path is not found", func() {
			config := &types.Config{
				WatchedConfigFilePath: "/home/config/config.json",
			}
			stopChan := make(chan struct{})
			err := initWatchConfig(config, stopChan)
			convey.So(err, convey.ShouldBeNil)
		})
	})
}
