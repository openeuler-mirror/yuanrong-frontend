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

package execendpoint

import (
	"sync"
	"testing"
)

func TestStorePutGetDelete(t *testing.T) {
	s := NewStore()
	if _, ok := s.Get("inst-1"); ok {
		t.Fatal("empty store should miss")
	}

	ep := Endpoint{InstanceID: "inst-1", ProxyGrpcAddress: "10.0.0.1:22774", ContainerID: "sbox-abc"}
	s.Put(ep)
	got, ok := s.Get("inst-1")
	if !ok {
		t.Fatal("expected hit after Put")
	}
	if got != ep {
		t.Fatalf("Get = %+v, want %+v", got, ep)
	}
	if s.Size() != 1 {
		t.Fatalf("Size = %d, want 1", s.Size())
	}

	s.Delete("inst-1")
	if _, ok := s.Get("inst-1"); ok {
		t.Fatal("expected miss after Delete")
	}
	if s.Size() != 0 {
		t.Fatalf("Size = %d, want 0", s.Size())
	}
}

func TestStorePutEmptyInstanceIDIgnored(t *testing.T) {
	s := NewStore()
	s.Put(Endpoint{InstanceID: "", ProxyGrpcAddress: "10.0.0.1:22774"})
	if s.Size() != 0 {
		t.Fatalf("Put with empty InstanceID should be ignored, Size = %d", s.Size())
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "inst"
			s.Put(Endpoint{InstanceID: id, ProxyGrpcAddress: "10.0.0.1:22774"})
			_, _ = s.Get(id)
			_ = s.Size()
			if n%2 == 0 {
				s.Delete(id)
			}
		}(i)
	}
	wg.Wait()
	// No assertion on final contents (racy by design); the test passes if -race
	// detects no data race and nothing panics.
}

func TestDefaultIsSingleton(t *testing.T) {
	if Default() != Default() {
		t.Fatal("Default() must return the same instance")
	}
}
