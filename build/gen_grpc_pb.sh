#!/bin/bash
# Copyright (c) Huawei Technologies Co., Ltd. 2025. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

BASE_DIR=$(
  cd "$(dirname "$0")"
  pwd
)
PROJECT_DIR="${BASE_DIR}"/../
DST_DIR="${PROJECT_DIR}"pkg/common/faas_common

function gen_code() {
  cd "${DST_DIR}"
  RPC_SERVICE_PROTO_PATH="${PROJECT_DIR}"pkg/common/faas_common/protobuf/
  if [ -d "${DST_DIR}/grpc" ]; then
    rm "${DST_DIR}/grpc" -rf
  fi

  protoc --proto_path="${RPC_SERVICE_PROTO_PATH}" \
    --go_out="${PROJECT_DIR}" --go_opt=module=frontend \
    --go-grpc_out="${PROJECT_DIR}" --go-grpc_opt=module=frontend \
    "${RPC_SERVICE_PROTO_PATH}"*.proto

  echo "generate gRPC: Done!"
}

function gen_posix_code() {
  cd "${DST_DIR}"
  RPC_PROTO_PATH="${PROJECT_DIR}"posix/proto/
  if [ ! -d "${RPC_PROTO_PATH}" ]; then
    echo "posix directory doesn't exist"
    return
  fi

  sed -i 's#"grpc/pb#"frontend/pkg/common/faas_common/grpc/pb#g' "${RPC_PROTO_PATH}"*.proto
  protoc --proto_path="${RPC_PROTO_PATH}" \
    --go_out="${PROJECT_DIR}" --go_opt=module=frontend \
    --go-grpc_out="${PROJECT_DIR}" --go-grpc_opt=module=frontend \
    "${RPC_PROTO_PATH}"*.proto

  echo "generate posix gRPC: Done!"
}

if [ -d "${DST_DIR}/grpc" ]; then
  rm "${DST_DIR}/grpc" -rf
fi
gen_code
gen_posix_code
exit 0