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

package leaseadaptor

import (
	"testing"

	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/logger/log"
)

const (
	testInitialBlueRatio = 50
	testUpdatedBlueRatio = 75
)

// TestInit tests the init function
func TestInit(t *testing.T) {
	if garyConfig == nil {
		t.Error("Expected garyConfig to be initialized, but it was nil")
	}
	if GetBlueRatio() != 0 {
		t.Errorf("Expected BlueRatio to be 0, but got %d", GetBlueRatio())
	}
}

// TestGetBlueRatio tests the GetBlueRatio function
func TestGetBlueRatio(t *testing.T) {
	garyConfig.BlueRatio.Store(testInitialBlueRatio)
	if ratio := GetBlueRatio(); ratio != testInitialBlueRatio {
		t.Errorf("Expected BlueRatio to be %d, but got %d", testInitialBlueRatio, ratio)
	}
}

// TestShouldUseBlueRing tests the ShouldUseBlueRing function
func TestShouldUseBlueRing(t *testing.T) {
	garyConfig.BlueRatio.Store(0)
	if ShouldUseBlueRing(0) {
		t.Errorf("Expected zero blue ratio to disable blue ring")
	}
	garyConfig.BlueRatio.Store(fullRatio)
	if !ShouldUseBlueRing(fullRatio) {
		t.Errorf("Expected full blue ratio to enable blue ring")
	}
	garyConfig.BlueRatio.Store(testInitialBlueRatio)
	if ShouldUseBlueRing(testInitialBlueRatio) {
		t.Errorf("Expected boundary hash to use green ring")
	}
}

// TestSetBlueRatio tests the SetBlueRatio function
func TestSetBlueRatio(t *testing.T) {
	event := &etcd3.Event{
		Value: []byte(`{"blue-ratio": "75%"}`),
	}

	SetBlueRatio(event, log.GetLogger())
	if GetBlueRatio() != testUpdatedBlueRatio {
		t.Errorf("Expected BlueRatio to be %d, but got %d", testUpdatedBlueRatio, GetBlueRatio())
	}

	// error
	event = &etcd3.Event{
		Value: []byte(`{"blueRatio": 50}`),
	}
	SetBlueRatio(event, log.GetLogger())
	if GetBlueRatio() != testUpdatedBlueRatio {
		t.Errorf("Expected BlueRatio to be %d, but got %d", testUpdatedBlueRatio, GetBlueRatio())
	}

	event = &etcd3.Event{
		Value: []byte(`{"blue-ratio": 50}`),
	}
	SetBlueRatio(event, log.GetLogger())
	if GetBlueRatio() != testUpdatedBlueRatio {
		t.Errorf("Expected BlueRatio to be %d, but got %d", testUpdatedBlueRatio, GetBlueRatio())
	}

	event = &etcd3.Event{
		Value: []byte(`{"blueRatio": "50%"}`),
	}
	SetBlueRatio(event, log.GetLogger())
	if GetBlueRatio() != testUpdatedBlueRatio {
		t.Errorf("Expected BlueRatio to be %d, but got %d", testUpdatedBlueRatio, GetBlueRatio())
	}
}

// TestClearBlueRatio tests the ClearBlueRatio function
func TestClearBlueRatio(t *testing.T) {
	garyConfig.BlueRatio.Store(testUpdatedBlueRatio)
	ClearBlueRatio()
	if GetBlueRatio() != 0 {
		t.Errorf("Expected BlueRatio to be 0, but got %d", GetBlueRatio())
	}
}
