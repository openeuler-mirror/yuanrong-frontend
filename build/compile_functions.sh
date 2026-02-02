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

# Log functions
log_error() {
    echo "[BUILD_ERROR][$(date +%b\ %d\ %H:%M:%S)]$*"
}

log_warning() {
    echo "[BUILD_WARN][$(date +%b\ %d\ %H:%M:%S)]$*"
}

function generate_pb() {
    # generate pb files
    if [ -z "${GOPATH}" ] || [ ! -d "${GOPATH}" ]; then
        log_error "GOPATH ${GOPATH} not exist!"
        return 1
    fi
    cd "${PROJECT_DIR}"/pkg
    [ -d "${GOPATH}/src/frontend" ] && rm -rf "${GOPATH}/src/frontend"
    mkdir -p "${GOPATH}"/src/
    ln -s "${PROJECT_DIR}" "${GOPATH}"/src/frontend
    if ! bash "${PROJECT_DIR}"/build/gen_grpc_pb.sh; then
        log_error "Failed to generate pb files!"
        return 1
    fi
}

go mod tidy
generate_pb