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

// Package config is used to keep the config used by the faas frontend function
package config

const (
	// HTTPServerListenPort is the listen port of frontend http server
	HTTPServerListenPort = 8888
	// ConfigFilePath defines config file path of frontend
	ConfigFilePath = "/home/sn/config/config.json"
	// ConfigEnvKey defines config env key of frontend
	ConfigEnvKey = "FRONTEND_CONFIG"
	// InitArgsFilePathEnvKey -
	InitArgsFilePathEnvKey = "INIT_ARGS_FILE_PATH"
)
