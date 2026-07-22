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

package server

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"

	"frontend/pkg/common/faas_common/etcd3"
	commonTls "frontend/pkg/common/faas_common/tls"
	mockUtils "frontend/pkg/common/faas_common/utils"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/middleware"
	"frontend/pkg/frontend/types"
)

func TestStart(t *testing.T) {
	prepareEnv()
	defer cleanEnv()
	defer gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
		return &types.Config{
			ClusterID: "cluster1",
			AzID:      "1",
			HTTPConfig: &types.FrontendHTTP{
				ServerListenPort: 8888,
			},
		}
	}).Reset()

	defer gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdRegister{}), "Register",
		func(_ *etcd3.EtcdRegister) error {
			return nil
		}).Reset()
	convey.Convey("Start", t, func() {
		stopCh := make(chan struct{})
		convey.Convey("success", func() {
			patches := []*gomonkey.Patches{
				gomonkey.ApplyMethod(reflect.TypeOf(&http.Server{}), "ListenAndServe",
					func(_ *http.Server) error {
						return nil
					}),
			}
			defer func() {
				for _, patch := range patches {
					patch.Reset()
				}
			}()
			err := Start(CreateHTTPServer(), stopCh)
			convey.So(err, convey.ShouldBeNil)
		})

		convey.Convey(" start https server success", func() {
			patches := []*gomonkey.Patches{
				gomonkey.ApplyMethod(reflect.TypeOf(&http.Server{}), "ListenAndServeTLS",
					func(_ *http.Server, certFile, keyFile string) error {
						return nil
					}),
				// ListenAndServeTLS(certFile, keyFile string) error
				gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
					return &types.Config{
						AzID:      "1",
						ClusterID: "1",
						HTTPSConfig: &commonTls.InternalHTTPSConfig{
							HTTPSEnable: true,
						},
						HTTPConfig: &types.FrontendHTTP{
							ServerListenPort: 8888,
						},
					}
				}),
				gomonkey.ApplyFunc(commonTls.InitTLSConfig, func(config commonTls.InternalHTTPSConfig) error {
					return nil
				}),

				gomonkey.ApplyFunc(commonTls.GetClientTLSConfig, func() *tls.Config {
					return &tls.Config{}
				}),
			}
			defer func() {
				for _, patch := range patches {
					patch.Reset()
				}
			}()
			server := CreateHTTPServer()
			err := Start(server, stopCh)
			convey.So(err, convey.ShouldBeNil)
			convey.So(server.TLSConfig, convey.ShouldNotBeNil)
		})

		convey.Convey("server failed", func() {
			patches := []*gomonkey.Patches{
				gomonkey.ApplyMethod(reflect.TypeOf(&http.Server{}), "ListenAndServe",
					func(_ *http.Server) error {
						return errors.New("server error")
					}),
			}
			defer func() {
				for _, patch := range patches {
					patch.Reset()
				}
			}()
			err := Start(CreateHTTPServer(), stopCh)
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("server failed exit", func() {
			rt := &mockUtils.FakeLibruntimeSdkClient{}
			exit := false
			patches := []*gomonkey.Patches{
				gomonkey.ApplyMethod(reflect.TypeOf(&http.Server{}), "ListenAndServe",
					func(_ *http.Server) error {
						return errors.New("server error")
					}),
				gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}), "Exit",
					func(_ *mockUtils.FakeLibruntimeSdkClient) {
						exit = true
					}),
			}
			defer func() {
				for _, patch := range patches {
					patch.Reset()
				}
			}()
			wait := make(chan int)
			go func() {
				err := Start(CreateHTTPServer(), stopCh)
				if err != nil {
					rt.Exit(0, "")
				}
				wait <- 1
			}()
			<-wait
			convey.So(exit, convey.ShouldBeTrue)

		})
	})
}

func TestGracefulShutdown(t *testing.T) {
	config.GetConfig().HTTPConfig = &types.FrontendHTTP{}
	config.GetConfig().HTTPConfig.ClientIdleTimeout = 0
	convey.Convey("GracefulShutdown", t, func() {
		convey.Convey("success", func() {
			patches := []*gomonkey.Patches{
				gomonkey.ApplyFunc(middleware.GraceExit, func() {}),
				gomonkey.ApplyFunc(os.Exit, func(code int) {}),
				gomonkey.ApplyMethod(reflect.TypeOf(&http.Server{}), "Shutdown", func(_ *http.Server,
					ctx context.Context) error {
					return nil
				}),
			}
			defer func() {
				for _, patch := range patches {
					patch.Reset()
				}
			}()
			stopCh := make(chan struct{})
			go func() {
				time.Sleep(100 * time.Millisecond)
				stopCh <- struct{}{}
			}()
			GracefulShutdown(&http.Server{})
		})
		convey.Convey("failed", func() {
			GracefulShutdown(&http.Server{})
		})
	})
}

func prepareEnv() {
	_ = os.Setenv("NODE_IP", "127.0.0.1")
	_ = os.Setenv("POD_NAME", "frontend_****")
	_ = os.Setenv("POD_IP", "127.0.0.1")
}

func cleanEnv() {
	_ = os.Setenv("NODE_IP", "")
	_ = os.Setenv("POD_NAME", "")
	_ = os.Setenv("POD_IP", "")
}
