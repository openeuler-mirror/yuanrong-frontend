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

// Package function function metadata sync with etcd
package functionmeta

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/client/v3"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/types"
)

func TestMetaData(t *testing.T) {
	metaInfo := types.FunctionMetaInfo{}
	value, _ := json.Marshal(metaInfo)
	err := ProcessUpdate("/tenant1//func1//version1", value, "meta")
	assert.NotNil(t, err)

	err = ProcessUpdate("//////tenant1//func1//version1", nil, "meta")
	assert.NotNil(t, err)

	err = ProcessUpdate("//////tenant1//func1//version1", value, "meta")
	assert.Equal(t, nil, err)

	err = ProcessUpdate("//////tenant1//func1//version1", value, "CAEMeta")
	assert.Equal(t, nil, err)

	loaded, ok := LoadFuncSpec("tenant1/func1/version1")
	assert.Equal(t, true, ok)
	assert.NotNil(t, loaded)

	err = ProcessDelete("/tenant1//func1//version1", "meta")
	assert.NotNil(t, err)

	err = ProcessDelete("//////tenant1//func1//version1", "CAEMeta")
	assert.Equal(t, nil, err)

	err = ProcessDelete("//////tenant1//func1//version1", "meta")
	assert.Equal(t, nil, err)

	err = ProcessDelete("//////tenant1//func1//version1", "CAEMeta")
	assert.Equal(t, nil, err)

	err = ProcessUpdate("//////tenant1//func2//version2", value, "CAEMeta")
	assert.Equal(t, nil, err)

	loaded2, ok := LoadFuncSpec("tenant1/func2/version2")
	assert.Equal(t, true, ok)
	assert.NotNil(t, loaded2)

	err = ProcessDelete("//////tenant1//func2//version2", "CAEMeta")
	assert.Equal(t, nil, err)
	etcdClient := &etcd3.EtcdClient{
		Client: &clientv3.Client{},
	}
	t.Run("load crMeta function notExists", func(t *testing.T) {
		enableAgentCRDRegistry = true
		defer gomonkey.ApplyFunc(etcd3.GetMetaEtcdClient, func() *etcd3.EtcdClient {
			return etcdClient
		}).Reset()
		defer gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdClient{}), "GetResponse",
			func(_ *etcd3.EtcdClient, ctxInfo etcd3.EtcdCtxInfo, etcdKey string,
				opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
				return &clientv3.GetResponse{
					Kvs: []*mvccpb.KeyValue{},
				}, nil
			}).Reset()
		defer func() {
			enableAgentCRDRegistry = false
		}()
		funcKey := "/tenant1//func-cr//version1"
		_, ok := LoadFuncSpec(funcKey)
		assert.False(t, ok)
	})
	t.Run("load crMeta function not funcSpec format", func(t *testing.T) {
		enableAgentCRDRegistry = true
		defer gomonkey.ApplyFunc(etcd3.GetMetaEtcdClient, func() *etcd3.EtcdClient {
			return etcdClient
		}).Reset()
		defer gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdClient{}), "GetResponse",
			func(_ *etcd3.EtcdClient, ctxInfo etcd3.EtcdCtxInfo, etcdKey string,
				opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
				return &clientv3.GetResponse{
					Kvs: []*mvccpb.KeyValue{},
				}, nil
			}).Reset()
		defer func() {
			funcCrSpecMap.Clear()
			enableAgentCRDRegistry = false
		}()
		funcKey := "/tenant1//func-cr//version2"
		funcCrSpecMap.Store(funcKey, 1)
		_, ok := LoadFuncSpec(funcKey)
		assert.False(t, ok)
	})
	t.Run("success to load crMeta function", func(t *testing.T) {
		enableAgentCRDRegistry = true
		defer func() {
			funcCrSpecMap.Clear()
			enableAgentCRDRegistry = false
		}()
		funcKey := "/tenant1//func-cr//version3"
		funcCrSpecMap.Store(funcKey, &types.FuncSpec{})
		_, ok := LoadFuncSpec(funcKey)
		assert.True(t, ok)
	})

	convey.Convey("Test funcSpecMap not exists", t, func() {
		convey.Convey("etcd meta not exist", func() {
			etcdClient := &etcd3.EtcdClient{
				Client: &clientv3.Client{},
			}
			defer gomonkey.ApplyFunc(etcd3.GetMetaEtcdClient, func() *etcd3.EtcdClient {
				return etcdClient
			}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdClient{}), "GetResponse",
				func(_ *etcd3.EtcdClient, ctxInfo etcd3.EtcdCtxInfo, etcdKey string,
					opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
					return &clientv3.GetResponse{
						Kvs: []*mvccpb.KeyValue{},
					}, nil
				}).Reset()
			_, ok = LoadFuncSpec("tenant1/func1/version1")
			assert.Equal(t, false, ok)
		})
		convey.Convey("etcd meta exist", func() {
			etcdClient := &etcd3.EtcdClient{
				Client: &clientv3.Client{},
			}
			defer gomonkey.ApplyFunc(etcd3.GetMetaEtcdClient, func() *etcd3.EtcdClient {
				return etcdClient
			}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdClient{}), "GetResponse",
				func(_ *etcd3.EtcdClient, ctxInfo etcd3.EtcdCtxInfo, etcdKey string,
					opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
					return &clientv3.GetResponse{
						Kvs: []*mvccpb.KeyValue{
							{
								Key:   []byte("test ok"),
								Value: []byte(`{"resourceMetaData":{"cpu":100,"memory":100}}`),
							},
						},
					}, nil
				}).Reset()
			_, ok = LoadFuncSpec("tenant1/func1/version2")
			assert.Equal(t, true, ok)
		})
	})

}

func TestFetchMetaEtcdWithSingleFlight(t *testing.T) {
	convey.Convey("Test FetchMetaEtcdWithSingleFlight", t, func() {
		convey.Convey("etcd client is nil", func() {
			etcdClient := &etcd3.EtcdClient{}
			defer gomonkey.ApplyFunc(etcd3.GetMetaEtcdClient, func() *etcd3.EtcdClient {
				return etcdClient
			}).Reset()
			funcKey := "123/testFunc/1"
			_, ok := fetchMetaEtcdWithSingleFlight(funcKey)
			assert.Equal(t, false, ok)
		})
		convey.Convey("get values error", func() {
			etcdClient := &etcd3.EtcdClient{
				Client: &clientv3.Client{},
			}
			defer gomonkey.ApplyFunc(etcd3.GetMetaEtcdClient, func() *etcd3.EtcdClient {
				return etcdClient
			}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdClient{}), "GetResponse",
				func(_ *etcd3.EtcdClient, ctxInfo etcd3.EtcdCtxInfo, etcdKey string,
					opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
					return nil, errors.New("error")
				}).Reset()
			funcKey := "123/testFunc/2"
			_, ok := fetchMetaEtcdWithSingleFlight(funcKey)
			assert.Equal(t, false, ok)
		})
		convey.Convey("value got from etcd is empty", func() {
			etcdClient := &etcd3.EtcdClient{
				Client: &clientv3.Client{},
			}
			var fetchEtcdTime int
			defer gomonkey.ApplyFunc(etcd3.GetMetaEtcdClient, func() *etcd3.EtcdClient {
				return etcdClient
			}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdClient{}), "GetResponse",
				func(_ *etcd3.EtcdClient, ctxInfo etcd3.EtcdCtxInfo, etcdKey string,
					opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
					fetchEtcdTime++
					return &clientv3.GetResponse{
						Kvs: []*mvccpb.KeyValue{},
					}, nil
				}).Reset()
			funcKey := "123/testFunc/3"
			_, ok := fetchMetaEtcdWithSingleFlight(funcKey)
			assert.Equal(t, false, ok)
			assert.Equal(t, 2, fetchEtcdTime)
		})
		convey.Convey("fetch success", func() {
			etcdClient := &etcd3.EtcdClient{
				Client: &clientv3.Client{},
			}
			defer gomonkey.ApplyFunc(etcd3.GetMetaEtcdClient, func() *etcd3.EtcdClient {
				return etcdClient
			}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdClient{}), "GetResponse",
				func(_ *etcd3.EtcdClient, ctxInfo etcd3.EtcdCtxInfo, etcdKey string,
					opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
					return &clientv3.GetResponse{
						Kvs: []*mvccpb.KeyValue{
							{
								Key:   []byte("test ok"),
								Value: []byte(`{"resourceMetaData":{"cpu":100,"memory":100}}`),
							},
						},
					}, nil
				}).Reset()
			funcKey := "123/testFunc/4"
			funcSpec, ok := fetchMetaEtcdWithSingleFlight(funcKey)
			assert.Equal(t, true, ok)
			convey.So(funcSpec.ResourceMetaData.CPU, convey.ShouldEqual, 100)
			convey.So(funcSpec.ResourceMetaData.Memory, convey.ShouldEqual, 100)
		})
		convey.Convey("fetch silent function success", func() {
			etcdClient := &etcd3.EtcdClient{
				Client: &clientv3.Client{},
			}
			defer gomonkey.ApplyFunc(etcd3.GetMetaEtcdClient, func() *etcd3.EtcdClient {
				return etcdClient
			}).Reset()
			defer gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdClient{}), "GetResponse",
				func(_ *etcd3.EtcdClient, ctxInfo etcd3.EtcdCtxInfo, etcdKey string,
					opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
					if strings.HasPrefix(etcdKey, "/silent") {
						return &clientv3.GetResponse{
							Kvs: []*mvccpb.KeyValue{
								{
									Key:   []byte("test ok"),
									Value: []byte(`{"resourceMetaData":{"cpu":100,"memory":100}}`),
								},
							},
						}, nil
					}
					return &clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{}}, nil
				}).Reset()
			funcKey := "123/testFunc/5"
			funcSpec, ok := fetchMetaEtcdWithSingleFlight(funcKey)
			assert.Equal(t, true, ok)
			convey.So(funcSpec.ResourceMetaData.CPU, convey.ShouldEqual, 100)
			convey.So(funcSpec.ResourceMetaData.Memory, convey.ShouldEqual, 100)
		})
	})
}

func TestLoadFuncSpecWithPath(t *testing.T) {
	routePrefix := "/hello"
	trie.Insert(strings.Split(routePrefix, constant.URLSeparator))
	funcRouteMap.Store(routePrefix, &types.FuncSpec{})
	spec, ok := LoadFuncSpecWithPath(routePrefix, "")
	assert.NotNil(t, spec)
	assert.Equal(t, true, ok)
	spec, ok = LoadFuncSpecWithPath("/hellos", "")
	assert.Nil(t, spec)
	assert.Equal(t, false, ok)
}

func TestUpdateRoute(t *testing.T) {
	currFuncSpec := &types.FuncSpec{
		ExtendedMetaData: types.ExtendedMetaData{
			ServeDeploySchema: types.ServeDeploySchema{
				Applications: []types.ServeApplicationSchema{{
					Name:        "testApp",
					RoutePrefix: "/hello",
				},
				},
			},
		},
	}
	updateRoute(nil, currFuncSpec)
	_, ok := funcRouteMap.Load("/hello")
	assert.Equal(t, true, ok)
	ok = trie.Search(strings.Split("/hello", constant.URLSeparator))
	assert.Equal(t, true, ok)

	preFuncSpec := &types.FuncSpec{
		ExtendedMetaData: types.ExtendedMetaData{
			ServeDeploySchema: types.ServeDeploySchema{
				Applications: []types.ServeApplicationSchema{{
					Name:        "testApp",
					RoutePrefix: "/hello",
				},
				},
			},
		},
	}
	currFuncSpec = &types.FuncSpec{
		ExtendedMetaData: types.ExtendedMetaData{
			ServeDeploySchema: types.ServeDeploySchema{
				Applications: []types.ServeApplicationSchema{{
					Name:        "testApp",
					RoutePrefix: "/world",
				},
				},
			},
		},
	}
	updateRoute(preFuncSpec, currFuncSpec)
	_, ok = funcRouteMap.Load("/hello")
	assert.Equal(t, false, ok)
	ok = trie.Search(strings.Split("/hello", constant.URLSeparator))
	assert.Equal(t, false, ok)

	_, ok = funcRouteMap.Load("/world")
	assert.Equal(t, true, ok)
	ok = trie.Search(strings.Split("/world", constant.URLSeparator))
	assert.Equal(t, true, ok)
}
