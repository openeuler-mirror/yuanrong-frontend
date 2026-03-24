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

package jwtauth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

// createValidJWT 创建一个有效的JWT token用于测试
func createValidJWT(header, payload, signature string) string {
	return header + "." + payload + "." + signature
}

// encodeBase64URL 将JSON对象编码为base64 URL编码字符串
func encodeBase64URL(obj interface{}) string {
	jsonBytes, _ := json.Marshal(obj)
	return base64.RawURLEncoding.EncodeToString(jsonBytes)
}

func TestParseJWT(t *testing.T) {
	tests := []struct {
		name        string
		authHeader  string
		wantErr     bool
		checkResult func(*testing.T, *ParsedJWT, error)
	}{
		{
			name:       "空字符串",
			authHeader: "",
			wantErr:    true,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.Nil(t, jwt)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "X-Auth header is empty")
			},
		},
		{
			name:       "无效格式-只有两部分",
			authHeader: "header.payload",
			wantErr:    true,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.Nil(t, jwt)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid JWT format")
			},
		},
		{
			name:       "无效格式-只有一部分",
			authHeader: "header",
			wantErr:    true,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.Nil(t, jwt)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid JWT format")
			},
		},
		{
			name:       "无效格式-超过三部分",
			authHeader: "header.payload.signature.extra",
			wantErr:    true,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.Nil(t, jwt)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid JWT format")
			},
		},
		{
			name:       "header base64解码失败",
			authHeader: createValidJWT("invalid-base64!!!", encodeBase64URL(JWTPayload{Sub: "tenant1", Exp: time.Now().Unix()}), "signature"),
			wantErr:    true,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.Nil(t, jwt)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to decode header")
			},
		},
		{
			name:       "payload base64解码失败",
			authHeader: createValidJWT(encodeBase64URL(JWTHeader{Alg: "HS256", Typ: "JWT"}), "invalid-base64!!!", "signature"),
			wantErr:    true,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.Nil(t, jwt)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to decode payload")
			},
		},
		{
			name:       "header JSON解析失败",
			authHeader: createValidJWT(base64.RawURLEncoding.EncodeToString([]byte("invalid json")), encodeBase64URL(JWTPayload{Sub: "tenant1", Exp: time.Now().Unix()}), "signature"),
			wantErr:    true,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.Nil(t, jwt)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to unmarshal header")
			},
		},
		{
			name:       "payload JSON解析失败",
			authHeader: createValidJWT(encodeBase64URL(JWTHeader{Alg: "HS256", Typ: "JWT"}), base64.RawURLEncoding.EncodeToString([]byte("invalid json")), "signature"),
			wantErr:    true,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.Nil(t, jwt)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to unmarshal payload")
			},
		},
		{
			name: "有效的JWT token-RawURL编码",
			authHeader: createValidJWT(
				encodeBase64URL(JWTHeader{Alg: "HS256", Typ: "JWT"}),
				encodeBase64URL(JWTPayload{Sub: "tenant123", Exp: 1234567890, Role: "developer"}),
				"signature123",
			),
			wantErr: false,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, jwt)
				assert.NotNil(t, jwt.Header)
				assert.Equal(t, "HS256", jwt.Header.Alg)
				assert.Equal(t, "JWT", jwt.Header.Typ)
				assert.NotNil(t, jwt.Payload)
				assert.Equal(t, "tenant123", jwt.Payload.Sub)
				assert.Equal(t, int64(1234567890), jwt.Payload.Exp)
				assert.Equal(t, "developer", jwt.Payload.Role)
				assert.Equal(t, "signature123", jwt.Signature)
			},
		},
		{
			name: "有效的JWT token-标准base64编码",
			authHeader: createValidJWT(
				base64.StdEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)),
				base64.StdEncoding.EncodeToString([]byte(`{"sub":"tenant456","exp":9876543210,"role":"user"}`)),
				"signature456",
			),
			wantErr: false,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, jwt)
				assert.NotNil(t, jwt.Header)
				assert.Equal(t, "HS256", jwt.Header.Alg)
				assert.Equal(t, "JWT", jwt.Header.Typ)
				assert.NotNil(t, jwt.Payload)
				assert.Equal(t, "tenant456", jwt.Payload.Sub)
				assert.Equal(t, int64(9876543210), jwt.Payload.Exp)
				assert.Equal(t, "user", jwt.Payload.Role)
				assert.Equal(t, "signature456", jwt.Signature)
			},
		},
		{
			name: "有效的JWT token-无role字段",
			authHeader: createValidJWT(
				encodeBase64URL(JWTHeader{Alg: "HS256", Typ: "JWT"}),
				encodeBase64URL(JWTPayload{Sub: "tenant789", Exp: 1111111111}),
				"signature789",
			),
			wantErr: false,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, jwt)
				assert.NotNil(t, jwt.Payload)
				assert.Equal(t, "tenant789", jwt.Payload.Sub)
				assert.Equal(t, int64(1111111111), jwt.Payload.Exp)
				assert.Equal(t, "", jwt.Payload.Role)
			},
		},
		{
			name: "有效的JWT token-exp为-1表示永不过期",
			authHeader: createValidJWT(
				encodeBase64URL(JWTHeader{Alg: "HS256", Typ: "JWT"}),
				encodeBase64URL(JWTPayload{Sub: "tenant-permanent", Exp: -1, Role: "developer"}),
				"signature-permanent",
			),
			wantErr: false,
			checkResult: func(t *testing.T, jwt *ParsedJWT, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, jwt)
				assert.NotNil(t, jwt.Payload)
				assert.Equal(t, "tenant-permanent", jwt.Payload.Sub)
				assert.Equal(t, int64(-1), jwt.Payload.Exp)
				assert.Equal(t, "developer", jwt.Payload.Role)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwt, err := ParseJWT(tt.authHeader)
			tt.checkResult(t, jwt, err)
		})
	}
}

func TestValidateTenantID(t *testing.T) {
	tests := []struct {
		name             string
		jwt              *ParsedJWT
		expectedTenantID string
		wantErr          bool
		checkResult      func(*testing.T, error)
	}{
		{
			name: "subject为空",
			jwt: &ParsedJWT{
				Payload: &JWTPayload{Sub: ""},
			},
			expectedTenantID: "tenant1",
			wantErr:          true,
			checkResult: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "subject (sub) is required")
			},
		},
		{
			name: "tenant ID匹配",
			jwt: &ParsedJWT{
				Payload: &JWTPayload{Sub: "tenant123"},
			},
			expectedTenantID: "tenant123",
			wantErr:          false,
			checkResult: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "tenant ID不匹配",
			jwt: &ParsedJWT{
				Payload: &JWTPayload{Sub: "tenant123"},
			},
			expectedTenantID: "tenant456",
			wantErr:          true,
			checkResult: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "tenant ID mismatch")
				assert.Contains(t, err.Error(), "expected tenant456")
				assert.Contains(t, err.Error(), "got tenant123")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.jwt.ValidateTenantID(tt.expectedTenantID)
			tt.checkResult(t, err)
		})
	}
}

func TestJWTPayloadIsExpired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name    string
		payload *JWTPayload
		want    bool
	}{
		{
			name:    "nil payload is treated as non-expiring",
			payload: nil,
			want:    false,
		},
		{
			name:    "exp minus one never expires",
			payload: &JWTPayload{Exp: -1},
			want:    false,
		},
		{
			name:    "exp zero never expires",
			payload: &JWTPayload{Exp: 0},
			want:    false,
		},
		{
			name:    "future exp is valid",
			payload: &JWTPayload{Exp: now.Unix() + 60},
			want:    false,
		},
		{
			name:    "past exp is expired",
			payload: &JWTPayload{Exp: now.Unix() - 60},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.payload.IsExpired(now))
		})
	}
}

// 使用goconvey的BDD风格测试
func TestParseJWT_Convey(t *testing.T) {
	convey.Convey("测试ParseJWT函数", t, func() {
		convey.Convey("当输入有效的JWT token时", func() {
			header := encodeBase64URL(JWTHeader{Alg: "HS256", Typ: "JWT"})
			payload := encodeBase64URL(JWTPayload{Sub: "test-tenant", Exp: time.Now().Unix(), Role: "developer"})
			authHeader := createValidJWT(header, payload, "test-signature")

			jwt, err := ParseJWT(authHeader)

			convey.So(err, convey.ShouldBeNil)
			convey.So(jwt, convey.ShouldNotBeNil)
			convey.So(jwt.Header.Alg, convey.ShouldEqual, "HS256")
			convey.So(jwt.Payload.Sub, convey.ShouldEqual, "test-tenant")
			convey.So(jwt.Payload.Role, convey.ShouldEqual, "developer")
		})

		convey.Convey("当输入空字符串时", func() {
			jwt, err := ParseJWT("")

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(jwt, convey.ShouldBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "X-Auth header is empty")
		})

		convey.Convey("当输入无效格式时", func() {
			jwt, err := ParseJWT("invalid.format")

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(jwt, convey.ShouldBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "invalid JWT format")
		})
	})
}

func TestValidateTenantID_Convey(t *testing.T) {
	convey.Convey("测试ValidateTenantID方法", t, func() {
		convey.Convey("当tenant ID匹配时", func() {
			jwt := &ParsedJWT{
				Payload: &JWTPayload{Sub: "tenant123"},
			}

			err := jwt.ValidateTenantID("tenant123")

			convey.So(err, convey.ShouldBeNil)
		})

		convey.Convey("当tenant ID不匹配时", func() {
			jwt := &ParsedJWT{
				Payload: &JWTPayload{Sub: "tenant123"},
			}

			err := jwt.ValidateTenantID("tenant456")

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "tenant ID mismatch")
		})

		convey.Convey("当subject为空时", func() {
			jwt := &ParsedJWT{
				Payload: &JWTPayload{Sub: ""},
			}

			err := jwt.ValidateTenantID("tenant123")

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "subject (sub) is required")
		})
	})
}
