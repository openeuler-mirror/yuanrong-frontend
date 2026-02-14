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

// Package functionmeta function metadata sync with cr
package functionmeta

import (
	"errors"
	"sync"

	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/leaseadaptor"
	"frontend/pkg/frontend/schedulerproxy"
	"frontend/pkg/frontend/subscriber"
)

var funcCrSpecMap sync.Map

// ProcessUpdateFromCr process update FuncSpec from cr
func ProcessUpdateFromCr(functionKey string, value []byte) error {
	currFuncSpec, err := buildFuncSpec(functionKey, value, "")
	if err != nil {
		return err
	}
	log.GetLogger().Infof("store new metadata: %s from cr", functionKey)
	funcCrSpecMap.Store(functionKey, currFuncSpec)
	sf.Remove(functionKey)
	subject.PublishEvent(subscriber.Update, currFuncSpec)
	return nil
}

// ProcessDeleteFromCr process delete FuncSpec
func ProcessDeleteFromCr(functionKey string) error {
	specValue, exist := funcCrSpecMap.Load(functionKey)
	if !exist {
		return nil
	}
	funcCrSpecMap.Delete(functionKey)
	spec, ok := specValue.(*types.FuncSpec)
	if !ok {
		return errors.New("not funcSpec type")
	}
	subject.PublishEvent(subscriber.Delete, spec)
	sf.Remove(functionKey)
	schedulerproxy.Proxy.DeleteBalancer(functionKey)
	log.GetLogger().Infof("delete function balancer :%s, from cr", functionKey)
	leaseadaptor.GetInstanceManager().ClearFuncLeasePools(functionKey)
	return nil
}
