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

package route

import (
	"errors"
	"net/url"
	"sync"
	"testing"
)

const (
	alternateSandboxPort = 9090
	concurrentIterations = 50
	concurrentWorkers    = 2
)

func target(id, safeID string, port uint16, host string) *Target {
	return &Target{
		Key:       Key{SafeInstanceID: safeID, Port: port},
		TargetURL: &url.URL{Scheme: "http", Host: host},
		Scheme:    "http",
	}
}

func TestCachePutGet(t *testing.T) {
	c := NewCache()
	c.PutInstance("inst-1", []*Target{target("inst-1", "inst-1", 8080, "10.0.0.1:31080")})

	got, err := c.Get(Key{SafeInstanceID: "inst-1", Port: 8080})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TargetURL.Host != "10.0.0.1:31080" {
		t.Errorf("host = %q", got.TargetURL.Host)
	}
}

func TestCacheMissing(t *testing.T) {
	c := NewCache()
	if _, err := c.Get(Key{SafeInstanceID: "x", Port: 1}); !errors.Is(err, ErrRouteNotFound) {
		t.Errorf("got %v, want ErrRouteNotFound", err)
	}
}

// PutInstance replaces the instance's prior routes wholesale.
func TestCacheReplaceInstanceRoutes(t *testing.T) {
	c := NewCache()
	c.PutInstance("inst-1", []*Target{target("inst-1", "inst-1", 8080, "h:1")})
	c.PutInstance("inst-1", []*Target{target("inst-1", "inst-1", alternateSandboxPort, "h:2")})

	if _, err := c.Get(Key{SafeInstanceID: "inst-1", Port: 8080}); !errors.Is(err, ErrRouteNotFound) {
		t.Error("old port 8080 should be gone after replace")
	}
	if _, err := c.Get(Key{SafeInstanceID: "inst-1", Port: alternateSandboxPort}); err != nil {
		t.Errorf("new port %d should be present: %v", alternateSandboxPort, err)
	}
}

// DeleteInstance removes every route the instance owns (multi-port).
func TestCacheDeleteInstance(t *testing.T) {
	c := NewCache()
	c.PutInstance("inst-1", []*Target{
		target("inst-1", "inst-1", 8080, "h:1"),
		target("inst-1", "inst-1", alternateSandboxPort, "h:2"),
	})
	c.DeleteInstance("inst-1")
	if c.Size() != 0 {
		t.Errorf("size = %d, want 0 after DeleteInstance", c.Size())
	}
}

// Collision edge: if two instances sanitize to the same key, the last writer
// owns it, and deleting the earlier instance must not remove the live route.
func TestCacheDeleteRespectsOwnership(t *testing.T) {
	c := NewCache()
	k := Key{SafeInstanceID: "shared", Port: 80}
	c.PutInstance("A", []*Target{{Key: k, TargetURL: &url.URL{Scheme: "http", Host: "a:1"}, Scheme: "http"}})
	c.PutInstance("B", []*Target{{Key: k, TargetURL: &url.URL{Scheme: "http", Host: "b:1"}, Scheme: "http"}})
	c.DeleteInstance("A")

	got, err := c.Get(k)
	if err != nil {
		t.Fatalf("route owned by B should remain: %v", err)
	}
	if got.TargetURL.Host != "b:1" {
		t.Errorf("host = %q, want b:1 (B owns the key)", got.TargetURL.Host)
	}
}

// Get returns a copy: mutating the result must not affect cached state.
func TestCacheGetReturnsCopy(t *testing.T) {
	c := NewCache()
	c.PutInstance("inst-1", []*Target{target("inst-1", "inst-1", 8080, "10.0.0.1:31080")})

	first, _ := c.Get(Key{SafeInstanceID: "inst-1", Port: 8080})
	first.TargetURL.Host = "mutated:0"
	first.Scheme = "mutated"

	second, _ := c.Get(Key{SafeInstanceID: "inst-1", Port: 8080})
	if second.TargetURL.Host != "10.0.0.1:31080" || second.Scheme != "http" {
		t.Errorf("cache mutated via returned copy: host=%q scheme=%q", second.TargetURL.Host, second.Scheme)
	}
}

// Run with -race to catch data races.
func TestCacheConcurrent(t *testing.T) {
	c := NewCache()
	var wg sync.WaitGroup
	for i := 0; i < concurrentIterations; i++ {
		wg.Add(concurrentWorkers)
		go func() {
			defer wg.Done()
			c.PutInstance("inst-1", []*Target{target("inst-1", "inst-1", 8080, "h:1")})
		}()
		go func() { defer wg.Done(); _, _ = c.Get(Key{SafeInstanceID: "inst-1", Port: 8080}) }()
	}
	wg.Wait()
}
