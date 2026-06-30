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

package state

import (
	"errors"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"

	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/state"
	"frontend/pkg/frontend/types"
)

func TestUpdateState(t *testing.T) {
	convey.Convey("Test updateState", t, func() {
		patch := gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
			return &etcd3.EtcdClient{}
		})
		defer patch.Reset()
		InitState()
		convey.Convey("frontendHandlerQueue is nil", func() {
			rawFq := frontendHandlerQueue
			frontendHandlerQueue = nil
			updateState(nil)
			frontendHandlerQueue = rawFq
			convey.So(reflect.DeepEqual(frontendState.Config, types.Config{}), convey.ShouldEqual, true)
		})
		convey.Convey("value is error type", func() {
			updateState("value")
			convey.So(reflect.DeepEqual(frontendState.Config, types.Config{}), convey.ShouldEqual, true)
		})
		convey.Convey("value is Config type", func() {
			q := &state.Queue{}
			patch := gomonkey.ApplyMethod(reflect.TypeOf(q),
				"SaveState", func(q *state.Queue, state []byte, key string) error {
					return nil
				})
			defer patch.Reset()
			config := &types.Config{}
			updateState(config)
			convey.So(frontendState.Config, convey.ShouldNotBeNil)
		})
		convey.Convey("save state error", func() {
			q := &state.Queue{}
			patch := gomonkey.ApplyMethod(reflect.TypeOf(q),
				"SaveState", func(q *state.Queue, state []byte, key string) error {
					return errors.New("save state error")
				})
			defer patch.Reset()
			config := &types.Config{}
			updateState(config)
			convey.So(frontendState.Config, convey.ShouldNotBeNil)
		})
	})
}

func TestGetStateByte(t *testing.T) {
	convey.Convey("Test getStateByte", t, func() {
		patch := gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
			return &etcd3.EtcdClient{}
		})
		defer patch.Reset()
		InitState()
		convey.Convey("frontendHandlerQueue is nil", func() {
			rawFq := frontendHandlerQueue
			frontendHandlerQueue = nil
			_, err := GetStateByte()
			frontendHandlerQueue = rawFq
			convey.So(err, convey.ShouldNotBeNil)
		})
		convey.SkipConvey("getStateByte success", func() {
			q := &state.Queue{}
			patch := gomonkey.ApplyMethod(reflect.TypeOf(q),
				"GetState", func(q *state.Queue, key string) ([]byte, error) {
					return []byte("state"), nil
				})
			convey.Reset(patch.Reset)
			stateBytes, err := GetStateByte()
			convey.So(string(stateBytes), convey.ShouldEqual, "state")
			convey.So(err, convey.ShouldBeNil)
		})
		convey.SkipConvey("GetState error", func() {
			q := &state.Queue{}
			patch := gomonkey.ApplyMethod(reflect.TypeOf(q),
				"GetState", func(q *state.Queue, key string) ([]byte, error) {
					return []byte{}, errors.New("get state error")
				})
			convey.Reset(patch.Reset)
			_, err := GetStateByte()
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestDeleteStateByte(t *testing.T) {
	convey.Convey("Test deleteState", t, func() {
		patch := gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
			return &etcd3.EtcdClient{}
		})
		defer patch.Reset()
		InitState()
		convey.Convey("frontendHandlerQueue is nil", func() {
			rawFq := frontendHandlerQueue
			frontendHandlerQueue = nil
			err := DeleteStateByte()
			frontendHandlerQueue = rawFq
			convey.So(err, convey.ShouldNotBeNil)
		})
		convey.Convey("DeleteState success", func() {
			q := &state.Queue{}
			patch := gomonkey.ApplyMethod(reflect.TypeOf(q),
				"DeleteState", func(q *state.Queue, key string) error {
					return nil
				})
			defer patch.Reset()
			err := DeleteStateByte()
			convey.So(err, convey.ShouldBeNil)
		})
	})
}

func TestUpdate(t *testing.T) {
	convey.Convey("Test Update", t, func() {
		patch := gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
			return &etcd3.EtcdClient{}
		})
		defer patch.Reset()
		InitState()
		convey.Convey("frontendHandlerQueue is nil", func() {
			rawFq := frontendHandlerQueue
			frontendHandlerQueue = nil
			Update("value")
			frontendHandlerQueue = rawFq
		})
		convey.Convey("update success", func() {
			Update("value")
		})
	})
}
