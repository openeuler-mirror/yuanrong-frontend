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

import "testing"

func TestDisplayInfoFormatsImageRootfs(t *testing.T) {
	info := DisplayInfo(`{"runtime":"runc","type":"image","imageurl":"registry.example.com/ns/image:tag"}`)

	if info.Image != "registry.example.com/ns/image:tag" {
		t.Fatalf("Image = %q, want registry.example.com/ns/image:tag", info.Image)
	}
	if info.Endpoint != "" {
		t.Fatalf("Endpoint = %q, want empty", info.Endpoint)
	}
}

func TestDisplayInfoFormatsS3RootfsWithEndpointWithoutSecrets(t *testing.T) {
	rootfs := `{"runtime":"runsc","type":"s3","imageurl":"registry.example.com","storageInfo":{"endpoint":"https://oss-cn-hangzhou.aliyuncs.com","bucket":"yr-rootfs-prod","object":"/images/python310/rootfs.img","accessKey":"secret-ak","secretKey":"secret-sk"}}`

	info := DisplayInfo(rootfs)

	if info.Image != "s3://yr-rootfs-prod/images/python310/rootfs.img" {
		t.Fatalf("Image = %q, want s3://yr-rootfs-prod/images/python310/rootfs.img", info.Image)
	}
	if info.Endpoint != "https://oss-cn-hangzhou.aliyuncs.com" {
		t.Fatalf("Endpoint = %q, want https://oss-cn-hangzhou.aliyuncs.com", info.Endpoint)
	}
	if info.Image == "secret-ak" || info.Endpoint == "secret-sk" {
		t.Fatalf("display info leaked secret fields: %+v", info)
	}
}

func TestDisplayInfoFormatsS3BucketOnly(t *testing.T) {
	info := DisplayInfo(`{"type":"s3","storageInfo":{"bucket":"crfs-dev"}}`)

	if info.Image != "s3://crfs-dev" {
		t.Fatalf("Image = %q, want s3://crfs-dev", info.Image)
	}
}

func TestDisplayInfoFallsBackToPlainLegacyRootfs(t *testing.T) {
	info := DisplayInfo("python:3.12-slim")

	if info.Image != "python:3.12-slim" {
		t.Fatalf("Image = %q, want python:3.12-slim", info.Image)
	}
}

func TestDisplayInfoIgnoresMalformedJSONRootfs(t *testing.T) {
	info := DisplayInfo(`{"type":"s3","storageInfo":`)

	if info.Image != "" || info.Endpoint != "" {
		t.Fatalf("malformed JSON display info = %+v, want empty", info)
	}
}
