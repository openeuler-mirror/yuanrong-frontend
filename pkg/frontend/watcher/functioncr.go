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

// Package watcher -
package watcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/k8sclient"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/faas_common/urnutils"
	"frontend/pkg/common/functioncr"
	"frontend/pkg/frontend/functionmeta"
)

const (
	// consistency with 1.x
	defaultMaxRetryTimes = 12

	defaultDoneChSize = 10
)

const (
	// SubEventTypeUpdate is update type of subscribe event
	SubEventTypeUpdate EventType = "update"
	// SubEventTypeDelete is delete type of subscribe event
	SubEventTypeDelete EventType = "delete"
	// SubEventTypeAdd is add type of subscribe event
	SubEventTypeAdd EventType = "add"
	// SubEventTypeSynced is synced type of subscribe event
	SubEventTypeSynced EventType = "synced"
)

// FunctionCRWatcher watches agent event of CR
type FunctionCRWatcher struct {
	dynamicClient   dynamic.Interface
	informerFactory dynamicinformer.DynamicSharedInformerFactory
	workQueue       workqueue.RateLimitingInterface

	lister   cache.GenericLister
	informer cache.SharedInformer
	synced   bool

	crKey2FuncKeyMap sync.Map

	crListDoneCh chan struct{}
	stopCh       <-chan struct{}
	sync.RWMutex
}

var waitQueue = make(chan crdRawEvent, 10000)

// EventType defines registry event type
type EventType string

// SubEvent contains event published to subscribers
type SubEvent struct {
	EventType
	EventMsg interface{}
}

// crdEvent include eventType and obj
type crdEvent struct {
	eventType EventType
	obj       *unstructured.Unstructured
}

type crdRawEvent struct {
	eventType    EventType
	objInterface interface{}
}

type crdSyncEvent struct {
	eventType EventType
	ch        chan struct{}
}

func startWatchFunctionCR(stopCh <-chan struct{}) {
	watcher := newFunctionCRWatcher(stopCh)
	if watcher != nil {
		watcher.Run()
	}
}

func newFunctionCRWatcher(stopCh <-chan struct{}) *FunctionCRWatcher {
	// prevent component startup exceptions when the YAML file for deployment permissions is not configured
	if os.Getenv(constant.EnableAgentCRDRegistry) == "" {
		log.GetLogger().Infof("not enable agent crd registry, skip")
		return nil
	}
	dynamicClient := k8sclient.GetDynamicClient()
	// Different CR events share the same rate-limiting queue
	workQueue := workqueue.NewNamedRateLimitingQueue(
		workqueue.DefaultControllerRateLimiter(),
		functioncr.CrdEventsQueue,
	)
	return &FunctionCRWatcher{
		dynamicClient:   dynamicClient,
		informerFactory: dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, time.Minute),
		workQueue:       workQueue,
		crListDoneCh:    make(chan struct{}, defaultDoneChSize),
		stopCh:          stopCh,
	}
}

// Run will start CR watch process
func (fw *FunctionCRWatcher) Run() {
	fw.informer = fw.informerFactory.ForResource(functioncr.GetCrdGVR()).Informer()
	fw.lister = fw.informerFactory.ForResource(functioncr.GetCrdGVR()).Lister()
	fw.setupEventHandlers(fw.informer)
	err := fw.initWatch(fw.informer)
	if err != nil {
		log.GetLogger().Errorf("failed to init function cr watcher, err: %s", err.Error())
	}
}

// setupEventHandlers setup CRD Event Handlers
func (fw *FunctionCRWatcher) setupEventHandlers(informer cache.SharedInformer) {
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			fw.RLock()
			defer fw.RUnlock()
			if fw.synced {
				fw.enqueueEvent(SubEventTypeAdd, obj)
			} else {
				waitQueue <- crdRawEvent{eventType: SubEventTypeAdd, objInterface: obj}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if oldObj.(*unstructured.Unstructured).GetResourceVersion() !=
				newObj.(*unstructured.Unstructured).GetResourceVersion() {
				fw.RLock()
				defer fw.RUnlock()
				if fw.synced {
					fw.enqueueEvent(SubEventTypeUpdate, newObj)
				} else {
					waitQueue <- crdRawEvent{eventType: SubEventTypeUpdate, objInterface: newObj}
				}

			}
		},
		DeleteFunc: func(obj interface{}) {
			fw.RLock()
			defer fw.RUnlock()
			if fw.synced {
				fw.enqueueEvent(SubEventTypeDelete, obj)
			} else {
				waitQueue <- crdRawEvent{eventType: SubEventTypeDelete, objInterface: obj}
			}
		},
	})
}

// enqueueEvent handle crd event enqueue
func (fw *FunctionCRWatcher) enqueueEvent(eventType EventType, objRaw interface{}) {
	if eventType == SubEventTypeSynced {
		ch, ok := objRaw.(chan struct{})
		if !ok {
			return
		}
		fw.workQueue.Add(&crdSyncEvent{eventType: SubEventTypeSynced, ch: ch})
		return
	}
	unstructObj, ok := objRaw.(*unstructured.Unstructured)
	if !ok {
		log.GetLogger().Errorf("failed to assert crd event")
		return
	}
	fw.workQueue.Add(&crdEvent{
		eventType: eventType,
		obj:       unstructObj,
	})
}

// initWatch start crd Controller
func (fw *FunctionCRWatcher) initWatch(informer cache.SharedInformer) error {
	ctx, _ := context.WithCancel(context.Background())
	go fw.informerFactory.Start(ctx.Done())
	go fw.processQueue()
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		log.GetLogger().Warnf("failed to sync function cr cache")
		return fmt.Errorf("wait for cache sync failed")
	}
	fw.Lock()
	defer fw.Unlock()
	fw.synced = true
	crRaws, err := fw.lister.List(labels.Everything())
	if err != nil {
		return nil
	}
	for _, crRaw := range crRaws {
		cr, ok := crRaw.(*unstructured.Unstructured)
		if ok {
			fw.enqueueEvent(SubEventTypeAdd, cr)
		}
	}

	waitQueueEventLen := len(waitQueue)
	for i := 0; i < waitQueueEventLen; i++ {
		event := <-waitQueue
		fw.enqueueEvent(event.eventType, event.objInterface)
	}
	fw.enqueueEvent(SubEventTypeSynced, fw.crListDoneCh)

	for {
		select {
		case _, ok := <-fw.crListDoneCh:
			if !ok {
				return fmt.Errorf("listen to list done ch failed")
			}
			log.GetLogger().Infof("finish to sync event")
			return nil
		}
	}
}

// processQueue process crd event Queue
func (fw *FunctionCRWatcher) processQueue() {
	for {
		item, shutdown := fw.workQueue.Get()
		if shutdown {
			return
		}
		syncEvent, ok := item.(*crdSyncEvent)
		if ok {
			log.GetLogger().Infof("recv cr sync event")
			handleSyncEvent(syncEvent.ch)
			fw.workQueue.Forget(item)
			fw.workQueue.Done(item)
			continue
		}
		event, ok := item.(*crdEvent)
		if !ok {
			log.GetLogger().Warnf("invalid crd event")
			fw.workQueue.Forget(item)
			fw.workQueue.Done(item)
			continue
		}
		if err := fw.processEvent(event); err != nil {
			// Limited number of retries
			if fw.workQueue.NumRequeues(item) < defaultMaxRetryTimes {
				log.GetLogger().Warnf("process crd event error: %s, retry", err.Error())
				fw.workQueue.AddRateLimited(item)
			}
		} else {
			fw.workQueue.Forget(item)
		}
		fw.workQueue.Done(item)
	}
}

// processEvent process cr add update delete Event
func (fw *FunctionCRWatcher) processEvent(event *crdEvent) error {
	crName := event.obj.GetName()
	namespace := event.obj.GetNamespace()
	logger := log.GetLogger().With(zap.Any("namespace", namespace)).With(zap.Any("crName", crName))
	objRaw, err := fw.lister.ByNamespace(namespace).Get(crName)
	if err != nil && !k8serrors.IsNotFound(err) { // 忽略这个错误
		log.GetLogger().Warnf("failed to list cr %s in namespace %s "+
			"and will ignore this error: %s", crName, namespace, err.Error())
		return nil
	}
	crKey := namespace + ":" + crName
	eventType := SubEventTypeUpdate
	if err != nil && k8serrors.IsNotFound(err) {
		eventType = SubEventTypeDelete
	}

	obj, ok := objRaw.(*unstructured.Unstructured)
	if !ok {
		return nil
	}

	switch eventType {
	case SubEventTypeUpdate:
		logger.Infof("recv function cr update event")
		err := fw.processFunctionCrUpdateEvent(crKey, obj)
		if err != nil {
			return err
		}
	case SubEventTypeDelete:
		logger.Infof("recv function cr delete event")
		err := fw.processFunctionCrDeleteEvent(crKey)
		if err != nil {
			return err
		}
	default:
		logger.Warnf("invalid event type")
	}
	return nil
}

func (fw *FunctionCRWatcher) processFunctionCrUpdateEvent(crKey string, obj *unstructured.Unstructured) error {
	functionSpecData, err := functioncr.GetFunctionSpecData(obj)
	if err != nil {
		return err
	}
	funcKey, err := getFunctionKey(crKey, functionSpecData)
	if err != nil {
		return err
	}
	if err := functionmeta.ProcessUpdateFromCr(funcKey, functionSpecData); err != nil {
		return err
	}
	fw.crKey2FuncKeyMap.Store(crKey, funcKey)
	return nil
}

func (fw *FunctionCRWatcher) processFunctionCrDeleteEvent(crKey string) error {
	logger := log.GetLogger().With(zap.Any("crKey", crKey))
	funcKeyVal, ok := fw.crKey2FuncKeyMap.Load(crKey)
	if !ok {
		logger.Warnf("not exists in crKey2FuncKeyMap, skip")
		return errors.New("not exists in crKey2FuncKeyMap")
	}
	funcKey, ok := funcKeyVal.(string)
	if !ok {
		logger.Warnf("failed to conver funckeyValue to string")
		return errors.New("funcKey is not string format")
	}
	if err := functionmeta.ProcessDeleteFromCr(funcKey); err != nil {
		return err
	}
	fw.crKey2FuncKeyMap.Delete(crKey)
	return nil
}

func getFunctionKey(crKey string, specBytes []byte) (string, error) {
	logger := log.GetLogger().With(zap.Any("crKey", crKey))
	var info = &types.FunctionMetaInfo{}
	if err := json.Unmarshal(specBytes, info); err != nil {
		logger.Errorf("failed to unmarshal crd spec: %s", err.Error())
		return "", err
	}
	logger = logger.With(zap.Any("urn", info.FuncMetaData.FunctionVersionURN))
	functionUrnInfo, err := urnutils.GetFunctionInfo(info.FuncMetaData.FunctionVersionURN)
	if err != nil {
		logger.Errorf("failed to convert urn, err: %s", err.Error())
		return "", err
	}
	funcKey := urnutils.CombineFunctionKey(functionUrnInfo.TenantID, functionUrnInfo.FuncName,
		functionUrnInfo.FuncVersion)
	return funcKey, nil
}

func handleSyncEvent(eventMsg interface{}) {
	ch, ok := eventMsg.(chan struct{})
	if !ok {
		return
	}
	if ch != nil {
		ch <- struct{}{}
	}
	return
}
