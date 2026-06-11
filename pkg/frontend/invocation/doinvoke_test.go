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

package invocation

import (
	"errors"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/statuscode"
	commonTypes "frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/functionmeta"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/types"
)

type innerCode int

func (c innerCode) String() string {
	switch {
	case c < 0:
		return "invalidCode"
	case c == 0:
		return "sysOK"
	case c < 10000:
		return "usrErr"
	default:
		return "sysErr"
	}
}

func Test_doInnerCodeString(t *testing.T) {
	msg := innerCode(-1).String()
	assert.Equal(t, msg, "invalidCode")
	msg = innerCode(0).String()
	assert.Equal(t, msg, "sysOK")
	msg = innerCode(100).String()
	assert.Equal(t, msg, "usrErr")
	msg = innerCode(10001).String()
	assert.Equal(t, msg, "sysErr")
}

func Test_prepareDynamicResource(t *testing.T) {
	type args struct {
		ctx *types.InvokeProcessContext
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]int64
		wantErr bool
	}{
		{"case1", args{ctx: &types.InvokeProcessContext{ReqHeader: map[string]string{constant.HeaderCPUSize: "600", constant.HeaderMemorySize: "512"}}},
			map[string]int64{constant.ResourceCPUName: 600, constant.ResourceMemoryName: 512}, false},
		{"case2", args{ctx: &types.InvokeProcessContext{}},
			map[string]int64{}, false},
		{"case3", args{ctx: &types.InvokeProcessContext{ReqHeader: map[string]string{
			constant.HeaderCustomResource: "{\"CPU\":1234567890}"}}},
			map[string]int64{constant.ResourceCPUName: 1234567890}, false},
		{"case4", args{ctx: &types.InvokeProcessContext{ReqHeader: map[string]string{
			constant.HeaderCustomResourceNew: "{\"CPU\":1234567890}"}}},
			map[string]int64{constant.ResourceCPUName: 1234567890}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prepareDynamicResource(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("prepareDynamicResource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("prepareDynamicResource() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getTimeout(t *testing.T) {
	funcSpecTimeout := int64(300)
	ctxTimeout := int64(60)
	timeout := getTimeout(funcSpecTimeout, ctxTimeout)
	assert.Equal(t, ctxTimeout, timeout)
	funcSpecTimeout = int64(300)
	ctxTimeout = int64(0)
	timeout = getTimeout(funcSpecTimeout, ctxTimeout)
	assert.Equal(t, funcSpecTimeout, timeout)
}

func TestInvokeHandler(t *testing.T) {
	setCode := 0
	defer gomonkey.ApplyFunc(responsehandler.SetErrorInContext, func(ctx *types.InvokeProcessContext, innerCode int,
		message interface{}) {
		setCode = innerCode
	}).Reset()
	defer gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
		return &types.Config{}
	}).Reset()
	convey.Convey("TestInvokeHandler", t, func() {
		convey.Convey("funcMeta not found", func() {
			defer gomonkey.ApplyFunc(functionmeta.LoadFuncSpec, func(funcKey string) (*commonTypes.FuncSpec, bool) {
				return nil, false
			}).Reset()
			ctx := &types.InvokeProcessContext{}
			err := InvokeHandler(ctx)
			convey.So(err, convey.ShouldNotBeEmpty)
			convey.So(setCode, convey.ShouldEqual, statuscode.FuncMetaNotFound)
		})
		convey.Convey("invoke error", func() {
			defer gomonkey.ApplyFunc(functionmeta.LoadFuncSpec, func(funcKey string) (*commonTypes.FuncSpec, bool) {
				return &commonTypes.FuncSpec{}, true
			}).Reset()
			defer gomonkey.ApplyPrivateMethod(reflect.TypeOf(&kernelRequestHandler{}), "invoke", func(_ *kernelRequestHandler) error {
				return errors.New("some error")
			}).Reset()
			ctx := &types.InvokeProcessContext{}
			err := InvokeHandler(ctx)
			convey.So(err, convey.ShouldNotBeEmpty)
		})
		convey.Convey("invoke success", func() {
			defer gomonkey.ApplyFunc(functionmeta.LoadFuncSpec, func(funcKey string) (*commonTypes.FuncSpec, bool) {
				return &commonTypes.FuncSpec{}, true
			}).Reset()
			defer gomonkey.ApplyPrivateMethod(reflect.TypeOf(&kernelRequestHandler{}), "invoke", func(_ *kernelRequestHandler) error {
				return nil
			}).Reset()
			ctx := &types.InvokeProcessContext{}
			err := InvokeHandler(ctx)
			convey.So(err, convey.ShouldBeNil)
		})
	})
}
