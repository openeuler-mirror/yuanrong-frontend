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

// Package snerror is basic information contained in the SN error.
package snerror

import (
	"encoding/json"
	"fmt"
)

const (
	// UserErrorMax is maximum value of user error
	UserErrorMax = 4999
	// UserErrorMin is minimal value of user error
	UserErrorMin = 4000
	// ErrorSeparator split error codes and error information.
	ErrorSeparator = "|"
)

// BadResponse HTTP request message that does not return 200
type BadResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// SNError defines the action contained in the SN error information.
type SNError interface {
	// Code Returned error code
	Code() int

	Error() string
}

type snError struct {
	code    int
	message string
}

// New returns an error.
// message is a complete English sentence with punctuation.
func New(code int, message string) SNError {
	return &snError{
		code:    code,
		message: message,
	}
}

// NewWithError err not nil.
func NewWithError(code int, err error) SNError {
	var message = ""
	if err != nil {
		message = err.Error()
	}
	return &snError{
		code:    code,
		message: message,
	}
}

// Code Returned error code
func (s *snError) Code() int {
	return s.code
}

// Error Implement the native error interface.
func (s *snError) Error() string {
	return s.message
}

// IsUserError true if a user error occurs
func IsUserError(s SNError) bool {
	// The user error is a four-digit integer.
	if UserErrorMin <= s.Code() && s.Code() <= UserErrorMax {
		return true
	}
	return false
}

// ConvertBadResponse converts a bad response body to SNError
func ConvertBadResponse(body []byte) SNError {
	if len(body) == 0 {
		return New(0, "empty response body")
	}
	var badResp BadResponse
	if err := json.Unmarshal(body, &badResp); err != nil {
		// If JSON parsing fails, return error with raw body as message
		return New(0, fmt.Sprintf("failed to parse error response: %s", string(body)))
	}
	// If code is 0 and message is empty, use the raw body as message
	if badResp.Code == 0 && badResp.Message == "" {
		return New(0, string(body))
	}
	return New(badResp.Code, badResp.Message)
}
