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

package rootfs

import (
	"encoding/json"
	"strings"
)

// Display contains non-sensitive rootfs fields suitable for list APIs.
type Display struct {
	Image    string
	Endpoint string
}

// DisplayInfo formats rootfs JSON or a legacy plain rootfs string for display.
// It intentionally ignores credentials and other secret storage fields.
func DisplayInfo(rootfs string) Display {
	rootfs = strings.TrimSpace(rootfs)
	if rootfs == "" {
		return Display{}
	}

	var parsed spec
	if err := json.Unmarshal([]byte(rootfs), &parsed); err == nil {
		return parsed.Display()
	}
	if !strings.HasPrefix(rootfs, "{") {
		return Display{Image: rootfs}
	}
	return Display{}
}

type spec struct {
	Type        string `json:"type"`
	ImageURL    string `json:"imageurl"`
	StorageInfo struct {
		Endpoint string `json:"endpoint"`
		Bucket   string `json:"bucket"`
		Object   string `json:"object"`
	} `json:"storageInfo"`
}

func (s spec) Display() Display {
	if strings.EqualFold(strings.TrimSpace(s.Type), "s3") {
		bucket := strings.TrimSpace(s.StorageInfo.Bucket)
		object := strings.Trim(strings.TrimSpace(s.StorageInfo.Object), "/")
		out := Display{Endpoint: strings.TrimSpace(s.StorageInfo.Endpoint)}
		if bucket != "" && object != "" {
			out.Image = "s3://" + bucket + "/" + object
			return out
		}
		if bucket != "" {
			out.Image = "s3://" + bucket
			return out
		}
		out.Image = strings.TrimSpace(s.ImageURL)
		return out
	}
	return Display{Image: strings.TrimSpace(s.ImageURL)}
}
