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

package responsehandler

import (
	"testing"

	"encoding/json"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCreateErrorResponseBody(t *testing.T) {
	Convey("Test CreateErrorResponseBody", t, func() {
		body, err := createErrorResponseBody(404, json.RawMessage(`"Not Found"`), "")
		want := []byte(`{"code":404,"message":"Not Found"}`)
		So(string(body), ShouldEqual, string(want))
		So(err, ShouldBeNil)
		body, err = createErrorResponseBody(0, json.RawMessage(`invalid json`), "")
		So(err, ShouldBeNil)
	})
}
