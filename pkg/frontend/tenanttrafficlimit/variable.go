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

package tenanttrafficlimit

import (
	"encoding/json"
	"math"
	"strings"

	"golang.org/x/time/rate"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/urnutils"
	"frontend/pkg/frontend/config"
)

const (
	tenantIDIndex      = 6
	zero               = 0
	defaultFrontendNum = 1
)

func (t *TenantContainer) getTenantLimiter(info TenantInfo) *rate.Limiter {
	if config.GetConfig().DefaultTenantLimitQuota == -1 {
		return rate.NewLimiter(rate.Inf, 0)
	}
	frontendNum := getFrontendNum()
	limitRate := float64(info.Quota) / float64(frontendNum)
	limitBucketSize := int(math.Ceil(float64(info.Quota)) /
		float64(frontendNum) * constant.TrafficRedundantRate)
	tenantLimiter := rate.NewLimiter(rate.Limit(limitRate), limitBucketSize)
	return tenantLimiter
}

func getFrontendNum() int {
	frontendNum := config.GetConfig().InstanceNum
	if frontendNum <= zero {
		frontendNum = defaultFrontendNum
	}
	return frontendNum
}

// ProcessUpdate -
func (t *TenantContainer) ProcessUpdate(event *etcd3.Event) error {
	var data map[string]int
	s := strings.Split(event.Key, constant.KeySeparator)
	if len(s) <= tenantIDIndex {
		log.GetLogger().Errorf("failed to get the tenantID")
		return nil
	}
	tenantID := s[tenantIDIndex]
	if err := json.Unmarshal(event.Value, &data); err != nil {
		log.GetLogger().Errorf("failed to unmarshal the etcd event value")
		return err
	}
	if data == nil {
		log.GetLogger().Errorf("failed to update the quota value")
		return nil
	}
	quota, ok := data["quota"]
	if !ok {
		log.GetLogger().Errorf("failed to get the quota value")
		return nil
	}
	tenantMsg := t.getTenantInfo(tenantID)
	tenantMsg.Quota = quota
	t.syncLimiter(tenantID, tenantMsg)
	log.GetLogger().Infof("update tenant %s quota update to %d.", urnutils.Anonymize(tenantID), quota)
	return nil
}

// ProcessDelete -
func (t *TenantContainer) ProcessDelete(event *etcd3.Event) {
	s := strings.Split(event.Key, constant.KeySeparator)
	if len(s) <= tenantIDIndex {
		log.GetLogger().Errorf("failed to get the tenantID")
		return
	}
	tenantID := s[tenantIDIndex]
	quota := config.GetConfig().DefaultTenantLimitQuota
	tenantMsg := t.getTenantInfo(tenantID)
	if tenantMsg.Limiter == nil {
		log.GetLogger().Errorf("tenantID limiter is not exit,no need to delete")
		return
	}
	tenantMsg.Quota = quota
	t.syncLimiter(tenantID, tenantMsg)
	log.GetLogger().Infof("delete tenant %s quota, and set it to default %d.",
		urnutils.Anonymize(tenantID), quota)
}

func getDefaultLimiter() *rate.Limiter {
	if config.GetConfig().DefaultTenantLimitQuota == -1 {
		return rate.NewLimiter(rate.Inf, 0)
	}
	frontendNum := getFrontendNum()
	limitRate := float64(config.GetConfig().DefaultTenantLimitQuota) / float64(frontendNum)
	limitBucketSize := int(math.Ceil(float64(config.GetConfig().DefaultTenantLimitQuota)) /
		float64(frontendNum) * constant.TrafficRedundantRate)
	tenantLimiter := rate.NewLimiter(rate.Limit(limitRate), limitBucketSize)
	return tenantLimiter
}
