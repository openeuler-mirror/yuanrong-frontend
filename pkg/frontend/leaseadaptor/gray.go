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
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"

	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/etcd3"
)

var (
	garyConfig *GrayConfig
	grayOnce   sync.Once
)

const (
	partLength   = 2
	blueRatioKey = "blue-ratio"
	fullRatio    = 100
)

func init() {
	grayOnce.Do(func() {
		garyConfig = &GrayConfig{}
	})
}

// GrayConfig -
type GrayConfig struct {
	BlueRatio atomic.Uint32
}

// GetBlueRatio -
func GetBlueRatio() uint32 {
	if garyConfig != nil {
		return garyConfig.BlueRatio.Load()
	}
	return 0
}

// ShouldUseBlueRing -
func ShouldUseBlueRing(hashPercent uint32) bool {
	blueRatio := GetBlueRatio()
	if blueRatio == 0 {
		return false
	}
	if blueRatio >= fullRatio {
		return true
	}
	return hashPercent < blueRatio
}

// SetBlueRatio -
func SetBlueRatio(event *etcd3.Event, logger api.FormatLogger) {
	var grayMap map[string]string
	err := json.Unmarshal(event.Value, &grayMap)
	if err != nil {
		logger.Errorf("unmarshal gray etcd event failed, value:%s, error: %s", event.Value, err)
		return
	}
	v, ok := grayMap[blueRatioKey]
	if !ok {
		logger.Errorf("get blue ratio failed, key is not %s", blueRatioKey)
		return
	}
	blueRatio, err := extractNum(v)
	if err != nil {
		logger.Errorf("set blue ratio failed, value:%s, error: %s", event.Value, err)
		return
	}
	if garyConfig != nil {
		garyConfig.BlueRatio.Store(blueRatio)
	}
	logger.Debugf("blue ratio: %d%", blueRatio)
}

func extractNum(percentStr string) (uint32, error) {
	re := regexp.MustCompile(`^(\d+)%?$`)
	matches := re.FindStringSubmatch(percentStr)
	if len(matches) < partLength {
		return 0, errors.New("failed to parse blue-ratio")
	}

	blueRatio, err := strconv.ParseUint(matches[1], 10, 32)
	if err != nil {
		return 0, errors.New("failed to parse uint32")
	}
	return uint32(blueRatio), nil
}

// ClearBlueRatio -
func ClearBlueRatio() {
	if garyConfig != nil {
		garyConfig.BlueRatio.Store(0)
	}
}
