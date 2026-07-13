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

package watcher

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/leaseadaptor"
	"frontend/pkg/frontend/types"
)

func TestGrayConfFilter(t *testing.T) {
	config.SetConfig(types.Config{
		ClusterID: "clusterXXX",
	})

	event1 := &etcd3.Event{Key: "/sn/faas-scheduler/gray"}
	assert.Equal(t, true, grayConfFilter(event1))

	event2 := &etcd3.Event{Key: "/sn/faas-scheduler/gray/clusterYYY"}
	assert.Equal(t, true, grayConfFilter(event2))

	event3 := &etcd3.Event{Key: "/sn/faas-scheduler/gray/clusterXXX"}
	assert.Equal(t, false, grayConfFilter(event3))
}

func TestGrayConfHandler(t *testing.T) {
	tests := []struct {
		name         string
		eventType    int
		expectedLogs []string
	}{
		{
			name:      "SYNCED event",
			eventType: etcd3.SYNCED,
			expectedLogs: []string{
				"faaSFrontend ready to receive etcd kv",
			},
		},
		{
			name:      "PUT event",
			eventType: etcd3.PUT,
			expectedLogs: []string{
				"recv gray event type",
				"SetBlueRatio called",
			},
		},
		{
			name:      "DELETE event",
			eventType: etcd3.DELETE,
			expectedLogs: []string{
				"recv gray event type",
				"clear blue ratio",
			},
		},
		{
			name:      "UNKNOWN event",
			eventType: 999,
			expectedLogs: []string{
				"recv gray event type",
				"unsupported event, type is 999",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &etcd3.Event{
				Key:   "/sn/faas-scheduler/gray/cluster1",
				Type:  tt.eventType,
				Rev:   123,
				Value: []byte(`{"blue-ratio": "75%"}`),
			}

			leaseadaptor.ClearBlueRatio()
			if tt.eventType == etcd3.DELETE {
				leaseadaptor.SetBlueRatio(event, log.GetLogger())
			}

			grayConfHandler(event)

			switch tt.eventType {
			case etcd3.PUT:
				assert.Equal(t, uint32(75), leaseadaptor.GetBlueRatio())
			case etcd3.DELETE:
				assert.Equal(t, uint32(0), leaseadaptor.GetBlueRatio())
			}
		})
	}
}
