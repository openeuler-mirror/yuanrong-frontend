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

package instanceconfigmanager

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"

	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/instanceconfig"
	"frontend/pkg/common/faas_common/logger/log"
)

func TestLoad(t *testing.T) {
	// Prepare test data
	funcKey := "test-func"
	invokeLabel := "test-label"
	mockConfig := &instanceconfig.Configuration{
		FuncKey:       funcKey,
		InstanceLabel: invokeLabel,
	}

	// Preload data
	manager.instanceConfigMaps[funcKey] = map[string]*instanceconfig.Configuration{
		invokeLabel: mockConfig,
	}
	defer func() {
		manager.instanceConfigMaps = make(map[string]map[string]*instanceconfig.Configuration)
	}()

	t.Run("Successfully load existing config", func(t *testing.T) {
		config, ok := Load(funcKey, invokeLabel)
		assert.True(t, ok)
		assert.Equal(t, mockConfig, config)
	})

	t.Run("Load non-existent funcKey", func(t *testing.T) {
		config, ok := Load("nonexistent", invokeLabel)
		assert.False(t, ok)
		assert.Nil(t, config)
	})

	t.Run("Load non-existent label", func(t *testing.T) {
		config, ok := Load(funcKey, "nonexistent")
		assert.False(t, ok)
		assert.Nil(t, config)
	})
}

func TestProcessUpdate(t *testing.T) {
	// Reset global state
	manager.instanceConfigMaps = make(map[string]map[string]*instanceconfig.Configuration)

	t.Run("Process new config addition", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		// Mock event publishing
		var publishCalled int
		patches.ApplyMethodFunc(subject, "PublishEvent", func(string, interface{}) {
			publishCalled++
		})

		// Execute test
		ProcessUpdate(&etcd3.Event{
			Key:   "/instances/business/yrk/cluster/cluster001/tenant/12345678901234561234567890123456/function/0@test111@yrfunc111/version/latest",
			Value: []byte("{\"instanceMetaData\":{\"maxInstance\":100,\"minInstance\":1,\"concurrentNum\":100,\"instanceType\":\"\",\"idleMode\":false,\"poolLabel\":\"\",\"poolId\":\"\"}}"),
		}, log.GetLogger())
		ProcessUpdate(&etcd3.Event{
			Key:   "/instances/business/yrk/cluster/cluster001/tenant/12345678901234561234567890123456/function/0@test111@yrfunc111/version/latest/label/aaa",
			Value: []byte("{\"instanceMetaData\":{\"maxInstance\":100,\"minInstance\":1,\"concurrentNum\":100,\"instanceType\":\"\",\"idleMode\":false,\"poolLabel\":\"\",\"poolId\":\"\"}}"),
		}, log.GetLogger())

		// Verify results
		assert.Equal(t, 1, len(manager.instanceConfigMaps))
		assert.Equal(t, 2, len(manager.instanceConfigMaps["default/0@test111@yrfunc111/latest"]))
		assert.Equal(t, 1, int(manager.instanceConfigMaps["default/0@test111@yrfunc111/latest"][""].InstanceMetaData.MinInstance))
		assert.Equal(t, 1, int(manager.instanceConfigMaps["default/0@test111@yrfunc111/latest"]["aaa"].InstanceMetaData.MinInstance))
		assert.Equal(t, publishCalled, 2)
	})

	t.Run("Config parse failure", func(t *testing.T) {
		manager.instanceConfigMaps = make(map[string]map[string]*instanceconfig.Configuration)
		ProcessUpdate(&etcd3.Event{
			Key:   "/instances/business/yrk/cluster/cluster001/tenant/12345678901234561234567890123456/function/0@test111@yrfunc111/version/latest/label",
			Value: []byte("{\"instanceMetaData\":{\"maxInstance\":100,\"minInstance\":1,\"concurrentNum\":100,\"instanceType\":\"\",\"idleMode\":false,\"poolLabel\":\"\",\"poolId\":\"\"}}"),
		}, log.GetLogger())
		ProcessUpdate(&etcd3.Event{
			Key:   "/instances/business/yrk/cluster/cluster001/tenant/12345678901234561234567890123456/function/0@test111@yrfunc111/version",
			Value: []byte("{\"instanceMetaData\":{\"maxInstance\":100,\"minInstance\":1,\"concurrentNum\":100,\"instanceType\":\"\",\"idleMode\":false,\"poolLabel\":\"\",\"poolId\":\"\"}}"),
		}, log.GetLogger())

		// Verify no new config was added
		assert.Equal(t, 0, len(manager.instanceConfigMaps))
	})
}

func TestProcessDelete(t *testing.T) {
	// Prepare test data
	funcKey := "default/0@test111@yrfunc111/latest"
	invokeLabel := "aaa"
	mockConfig := &instanceconfig.Configuration{
		FuncKey:       funcKey,
		InstanceLabel: invokeLabel,
	}
	mockConfigEmptyLabel := &instanceconfig.Configuration{
		FuncKey:       funcKey,
		InstanceLabel: "",
	}

	// Preload data
	manager.instanceConfigMaps[funcKey] = map[string]*instanceconfig.Configuration{
		invokeLabel: mockConfig,
		"":          mockConfigEmptyLabel,
	}

	t.Run("Successfully delete config", func(t *testing.T) {
		// Mock parse result
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		// Mock event publishing
		var publishCalled bool
		patches.ApplyMethodFunc(subject, "PublishEvent", func(string, interface{}) {
			publishCalled = true
		})

		// Execute test
		ProcessDelete(&etcd3.Event{
			Key:       "/instances/business/yrk/cluster/cluster001/tenant/12345678901234561234567890123456/function/0@test111@yrfunc111/version/latest/label/aaa",
			Value:     []byte("{\"instanceMetaData\":{\"maxInstance\":100,\"minInstance\":1,\"concurrentNum\":100,\"instanceType\":\"\",\"idleMode\":false,\"poolLabel\":\"\",\"poolId\":\"\"}}"),
			PrevValue: []byte("{\"instanceMetaData\":{\"maxInstance\":100,\"minInstance\":1,\"concurrentNum\":100,\"instanceType\":\"\",\"idleMode\":false,\"poolLabel\":\"\",\"poolId\":\"\"}}"),
		}, log.GetLogger())

		// Verify results
		assert.Equal(t, 1, len(manager.instanceConfigMaps[funcKey]))
		assert.True(t, publishCalled)
	})

	t.Run("Delete non-existent config", func(t *testing.T) {
		manager.instanceConfigMaps[funcKey] = map[string]*instanceconfig.Configuration{
			invokeLabel: mockConfig,
			"":          mockConfigEmptyLabel,
		}
		// Mock parse returns non-existent funcKey
		patches := gomonkey.NewPatches()
		defer patches.Reset()

		ProcessDelete(&etcd3.Event{
			Key:       "/nonexistent/path",
			PrevValue: []byte("invalid-data"),
		}, log.GetLogger())

		// Verify original data remains unchanged
		assert.Equal(t, 2, len(manager.instanceConfigMaps[funcKey]))
	})
}

func TestGlobalInstanceConfigManager(t *testing.T) {
	// Test global singleton
	assert.NotNil(t, manager)
}
