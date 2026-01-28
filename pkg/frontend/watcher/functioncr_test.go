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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/functioncr"
)

func TestNewFunctionCRWatcher(t *testing.T) {
	stopCh := make(chan struct{})
	t.Run("enable agent crd registry", func(t *testing.T) {
		os.Setenv(constant.EnableAgentCRDRegistry, "true")
		defer os.Setenv(constant.EnableAgentCRDRegistry, "")
		watcher := newFunctionCRWatcher(stopCh)
		assert.NotNil(t, watcher, "failed to new function cr watcher")
	})
	t.Run("not enable agent crd registry", func(t *testing.T) {
		os.Setenv(constant.EnableAgentCRDRegistry, "")
		watcher := newFunctionCRWatcher(stopCh)
		assert.Nil(t, watcher)
	})
}

// createTestCR 创建一个测试用的自定义资源
func createTestCR(name, functionUrn string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "yr.cap.io/v1",
			"kind":       "YRTask",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         "default",
				"uid":               "1234-5678-90ab-cdef-" + name,
				"resourceVersion":   "1",
				"creationTimestamp": metav1.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"funcMetaData": map[string]interface{}{
					"functionURN": functionUrn,
				},
				"extendedMetaData": map[string]interface{}{
					"customContainerConfig": map[string]interface{}{
						"image": "nginx:latest",
					},
				},
			},
		},
	}
}

var (
	// AgentList 资源的 GVK
	agentListGVK = schema.GroupVersionKind{
		Group:   "agentrun.cap.io",
		Version: "v1",
		Kind:    "YRTaskList",
	}
)

// TestControllerWithPrePopulatedData 验证在 Informer 启动前预填充的数据
func TestControllerWithPrePopulatedData(t *testing.T) {
	// 1️⃣ 创建 fake client
	scheme := runtime.NewScheme()
	// 添加必要的类型到 scheme
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add appsv1 to scheme: %v", err)
	}
	listResourceMap := map[schema.GroupVersionResource]string{
		functioncr.GetCrdGVR(): agentListGVK.Kind,
	}
	//fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)
	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listResourceMap,
		createTestCR("init-cr-1", "func1"), createTestCR("init-cr-2", "func2"))
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(fakeClient, 0, corev1.NamespaceAll, nil)

	stopCh := make(chan struct{})
	defer close(stopCh)
	os.Setenv(constant.EnableAgentCRDRegistry, "on")
	functionCRWatcher := newFunctionCRWatcher(stopCh)
	functionCRWatcher.dynamicClient = fakeClient
	functionCRWatcher.informerFactory = factory
	functionCRWatcher.Run()
}

func TestFunctionCRWatcher_processFunctionCrDeleteEvent(t *testing.T) {
	fw := FunctionCRWatcher{}
	t.Run("crKey not exists", func(t *testing.T) {
		err := fw.processFunctionCrDeleteEvent("default:notExists")
		assert.EqualError(t, err, "not exists in crKey2FuncKeyMap")
	})
	t.Run("funckey is not string", func(t *testing.T) {
		fw.crKey2FuncKeyMap.Store("default:yin", 1)
		defer fw.crKey2FuncKeyMap.Clear()
		err := fw.processFunctionCrDeleteEvent("default:yin")
		assert.EqualError(t, err, "funcKey is not string format")
	})
	t.Run("success", func(t *testing.T) {
		fw.crKey2FuncKeyMap.Store("default:yin", "funcKey")
		defer fw.crKey2FuncKeyMap.Clear()
		err := fw.processFunctionCrDeleteEvent("default:yin")
		assert.NoError(t, err)
		_, ok := fw.crKey2FuncKeyMap.Load("default:yin")
		assert.False(t, ok)
	})
}
