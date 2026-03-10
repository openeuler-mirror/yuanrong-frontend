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

package asyncinvocation

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

func TestAsyncResultStore_StoreAndLoad(t *testing.T) {
	convey.Convey("Store and Load async result", t, func() {
		store := &AsyncResultStore{}
		result := &AsyncResult{
			RequestID: "req-001",
			Status:    StatusPending,
			CreatedAt: time.Now(),
		}
		store.Store("req-001", result)

		convey.Convey("should load existing result", func() {
			loaded, ok := store.Load("req-001")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(loaded.RequestID, convey.ShouldEqual, "req-001")
			convey.So(loaded.Status, convey.ShouldEqual, StatusPending)
		})

		convey.Convey("should return false for non-existing result", func() {
			_, ok := store.Load("req-999")
			convey.So(ok, convey.ShouldBeFalse)
		})
	})
}

func TestAsyncResultStore_Delete(t *testing.T) {
	convey.Convey("Delete async result", t, func() {
		store := &AsyncResultStore{}
		store.Store("req-002", &AsyncResult{
			RequestID: "req-002",
			Status:    StatusCompleted,
			CreatedAt: time.Now(),
		})

		store.Delete("req-002")
		_, ok := store.Load("req-002")
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func TestAsyncResultStore_Cleanup(t *testing.T) {
	convey.Convey("Cleanup expired results", t, func() {
		store := &AsyncResultStore{}
		store.Store("old", &AsyncResult{
			RequestID: "old",
			Status:    StatusCompleted,
			CreatedAt: time.Now().Add(-2 * time.Hour),
		})
		store.Store("new", &AsyncResult{
			RequestID: "new",
			Status:    StatusCompleted,
			CreatedAt: time.Now(),
		})

		store.StartCleanup(50*time.Millisecond, 1*time.Hour)
		time.Sleep(150 * time.Millisecond)

		_, oldExists := store.Load("old")
		_, newExists := store.Load("new")
		convey.So(oldExists, convey.ShouldBeFalse)
		convey.So(newExists, convey.ShouldBeTrue)
	})
}

func TestAsyncResultStore_StatusTransitions(t *testing.T) {
	convey.Convey("Status transitions", t, func() {
		store := &AsyncResultStore{}
		result := &AsyncResult{
			RequestID: "req-003",
			Status:    StatusPending,
			CreatedAt: time.Now(),
		}
		store.Store("req-003", result)

		convey.Convey("pending -> running -> completed", func() {
			result.Status = StatusRunning
			loaded, _ := store.Load("req-003")
			convey.So(loaded.Status, convey.ShouldEqual, StatusRunning)

			now := time.Now()
			result.Status = StatusCompleted
			result.StatusCode = 200
			result.RespBody = []byte("ok")
			result.CompletedAt = &now
			loaded, _ = store.Load("req-003")
			convey.So(loaded.Status, convey.ShouldEqual, StatusCompleted)
			convey.So(loaded.StatusCode, convey.ShouldEqual, 200)
			convey.So(loaded.CompletedAt, convey.ShouldNotBeNil)
		})
	})
}
