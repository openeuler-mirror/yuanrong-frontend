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

package lease

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/smartystreets/goconvey/convey"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/grpc/pb/lease"
	"frontend/pkg/common/faas_common/types"
	mockUtils "frontend/pkg/common/faas_common/utils"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/remoteclientlease"
)

var in = &types.InstanceInfo{
	TenantID:     "test-TenantID",
	FunctionName: "test-faasmanager",
	Version:      "",
	InstanceName: "test-faasnamager-instance",
}

type KvMock struct {
}

func (k *KvMock) Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	return nil, nil
}

func (k *KvMock) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	response := &clientv3.GetResponse{}
	response.Count = 10
	return response, nil
}

func (k *KvMock) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KvMock) Compact(ctx context.Context, rev int64, opts ...clientv3.CompactOption) (*clientv3.CompactResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KvMock) Do(ctx context.Context, op clientv3.Op) (clientv3.OpResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (k *KvMock) Txn(ctx context.Context) clientv3.Txn {
	//TODO implement me
	panic("implement me")
}

var event = etcd3.Event{
	Type: etcd3.PUT,
	Key:  "/sn/instance/business/yrk/tenant/1/function/test-faasmanager/version/latest/defaultaz/requestID/test-faasnamager-instance",
	Value: []byte(`{
    "instanceID": "test-faasnamager-instance",
    "instanceStatus": {
        "code": 3,
        "msg": "running"
    }}`)}

func TestNewLeaseHandler(t *testing.T) {
	util.SetAPIClientLibruntime(&mockUtils.FakeLibruntimeSdkClient{})
	defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetAsync",
		func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetAsyncCallback) {
			cb([]byte{}, nil)
		}).Reset()
	defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetEvent",
		func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetEventCallback) {
			cb([]byte{}, nil)
		}).Reset()
	defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "DeleteGetEventCallback",
		func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string) {
			return
		}).Reset()
	convey.Convey("NewLeaseHandler", t, func() {
		convey.Convey("failed to parse lease request, empty remote client id", func() {
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add(constant.HeaderTraceID, "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			NewLeaseHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
		convey.Convey("failed to unmarshal msg", func() {
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add(constant.HeaderTraceID, "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			defer gomonkey.ApplyFunc(proto.Unmarshal, func(b []byte, m proto.Message) error {
				return errors.New("proto unmarshal error")
			}).Reset()
			NewLeaseHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
		convey.Convey("invoke failed", func() {
			defer gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
				return &etcd3.EtcdClient{Client: &clientv3.Client{KV: &KvMock{}}}
			}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetAsync",
				func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetAsyncCallback) {
					cb([]byte{}, errors.New("invoke failed"))
				}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetEvent",
				func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetEventCallback) {
					cb([]byte{}, nil)
				}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "DeleteGetEventCallback",
				func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string) {
					return
				}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add(constant.HeaderTraceID, "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			NewLeaseHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
		})
		convey.Convey("OK", func() {
			p := gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
				return &etcd3.EtcdClient{Client: &clientv3.Client{KV: &KvMock{}}}
			})
			defer p.Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add(constant.HeaderTraceID, "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			NewLeaseHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
		})
		convey.Convey("internal error", func() {
			p := gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
				return &etcd3.EtcdClient{Client: &clientv3.Client{KV: &KvMock{}}}
			})
			defer p.Reset()
			p2 := gomonkey.ApplyFunc((*KvMock).Put, func(_ *KvMock, ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
				return nil, fmt.Errorf("error")
			})
			defer p2.Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			patch := gomonkey.ApplyFunc(proto.Marshal,
				func(m proto.Message) ([]byte, error) {
					return nil, errors.New("some error")
				})
			defer patch.Reset()
			NewLeaseHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
	})
}

func TestDelLeaseHandler(t *testing.T) {
	util.SetAPIClientLibruntime(&mockUtils.FakeLibruntimeSdkClient{})
	defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetAsync",
		func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetAsyncCallback) {
			cb([]byte{}, nil)
		}).Reset()
	defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetEvent",
		func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetEventCallback) {
			cb([]byte{}, nil)
		}).Reset()
	defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "DeleteGetEventCallback",
		func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string) {
			return
		}).Reset()
	convey.Convey("DelLeaseHandler", t, func() {
		convey.Convey("failed to parse lease request, empty remote client id", func() {
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add("traceId", "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			DelLeaseHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
		convey.Convey("failed to unmarshal msg", func() {
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add("traceId", "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			defer gomonkey.ApplyFunc(proto.Unmarshal, func(b []byte, m proto.Message) error {
				return errors.New("proto unmarshal error")
			}).Reset()
			DelLeaseHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
		convey.Convey("invoke failed", func() {
			defer gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
				return &etcd3.EtcdClient{Client: &clientv3.Client{KV: &KvMock{}}}
			}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetAsync",
				func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetAsyncCallback) {
					cb([]byte{}, errors.New("invoke failed"))
				}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetEvent",
				func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetEventCallback) {
					cb([]byte{}, nil)
				}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "DeleteGetEventCallback",
				func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string) {
					return
				}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add("traceId", "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			DelLeaseHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
		})
		convey.Convey("OK", func() {
			p := gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
				return &etcd3.EtcdClient{Client: &clientv3.Client{KV: &KvMock{}}}
			})
			defer p.Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add("traceId", "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			DelLeaseHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
		})
		convey.Convey("internal error", func() {
			p := gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
				return &etcd3.EtcdClient{Client: &clientv3.Client{KV: &KvMock{}}}
			})
			defer p.Reset()
			p2 := gomonkey.ApplyFunc((*KvMock).Put, func(_ *KvMock, ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
				return nil, fmt.Errorf("error")
			})
			defer p2.Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			patch := gomonkey.ApplyFunc(proto.Marshal,
				func(m proto.Message) ([]byte, error) {
					return nil, errors.New("some error")
				})
			defer patch.Reset()
			DelLeaseHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
	})
}

func TestKeepAliveHandler(t *testing.T) {
	util.SetAPIClientLibruntime(&mockUtils.FakeLibruntimeSdkClient{})
	defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetAsync",
		func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetAsyncCallback) {
			cb([]byte{}, nil)
		}).Reset()
	defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetEvent",
		func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetEventCallback) {
			cb([]byte{}, nil)
		}).Reset()
	defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "DeleteGetEventCallback",
		func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string) {
			return
		}).Reset()
	convey.Convey("KeepAliveHandler", t, func() {
		convey.Convey("failed to parse lease request, empty remote client id", func() {
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add("traceId", "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			KeepAliveHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
		convey.Convey("failed to unmarshal msg", func() {
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add("traceId", "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			defer gomonkey.ApplyFunc(proto.Unmarshal, func(b []byte, m proto.Message) error {
				return errors.New("proto unmarshal error")
			}).Reset()
			KeepAliveHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
		convey.Convey("invoke failed", func() {
			defer gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
				return &etcd3.EtcdClient{Client: &clientv3.Client{KV: &KvMock{}}}
			}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetAsync",
				func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetAsyncCallback) {
					cb([]byte{}, errors.New("invoke failed"))
				}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "GetEvent",
				func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string, cb api.GetEventCallback) {
					cb([]byte{}, nil)
				}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "DeleteGetEventCallback",
				func(_ *mockUtils.FakeLibruntimeSdkClient, objectID string) {
					return
				}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add("traceId", "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			KeepAliveHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
		})
		convey.Convey("OK", func() {
			p := gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
				return &etcd3.EtcdClient{Client: &clientv3.Client{KV: &KvMock{}}}
			})
			defer p.Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			ctx.Request.Header.Add("traceId", "test-traceID")
			remoteclientlease.UpdateFaasManager(&event, in)
			KeepAliveHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
		})
		convey.Convey("internal error", func() {
			p := gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
				return &etcd3.EtcdClient{Client: &clientv3.Client{KV: &KvMock{}}}
			})
			defer p.Reset()
			p2 := gomonkey.ApplyFunc((*KvMock).Put, func(_ *KvMock, ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
				return nil, fmt.Errorf("error")
			})
			defer p2.Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			msg := &lease.LeaseRequest{
				RemoteClientId: "test-clientID",
			}
			body, _ := proto.Marshal(msg)
			ctx.Request, _ = http.NewRequest("", "", bytes.NewBuffer(body))
			patch := gomonkey.ApplyFunc(proto.Marshal,
				func(m proto.Message) ([]byte, error) {
					return nil, errors.New("some error")
				})
			defer patch.Reset()
			KeepAliveHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
	})
}
