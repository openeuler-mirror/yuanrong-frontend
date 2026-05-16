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

# global environment
CUR_DIR=$(dirname "$(readlink -f "$0")")
ROOT_PATH=${CUR_DIR}/../..
SRC_PATH=${ROOT_PATH}/pkg/frontend
OUTPUT_PATH=${CUR_DIR}/output
echo LD_LIBRARY_PATH=$LD_LIBRARY_PATH

go mod tidy
ARCH="x86_64"
# run go test and report
run_gocover_report()
{
    rm -rf "${OUTPUT_PATH}"
    mkdir -p "${OUTPUT_PATH}"

    cd ${SRC_PATH}
    find frontendsdk/ -type f -print0 | xargs -0 sed -i 's#"yuanrong.org/kernel/runtime/posixsdk"#"functionsystem/pkg/frontend/frontendsdk/posixsdk"#g'
    go test -tags function -gcflags=all=-l -covermode="${GOCOVER_MODE}" -coverprofile="$OUTPUT_PATH/faasfrontend.cover" -coverpkg="./..." "./..."

    if [ $? -ne 0 ]; then
        log_error "failed to go test faasfrontend"
        exit 1
    fi

    # export llt coverage result
    cd "$OUTPUT_PATH"
    echo "mode: ${GOCOVER_MODE}" > coverage.out && cat ./*.cover | grep -v mode: | grep -v pb.go | grep -v fb.go | sort -r | \
    awk '{if($1 != last) {print $0;last=$1}}' >> coverage.out

    gocov convert coverage.out > coverage.json
    gocov report coverage.json > CoverResult.txt
    gocov-html coverage.json > coverage.html
}

run_gocover_report
exit 0