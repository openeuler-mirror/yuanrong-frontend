/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2026. All rights reserved.
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

package sshproxy

import (
	"fmt"

	"frontend/pkg/common/faas_common/types"
)

type keyPrincipal struct {
	subject  string
	allowAll bool
}

func (p keyPrincipal) authorizeRequestedTarget(route) error {
	if p.allowAll {
		return nil
	}
	return fmt.Errorf("SSH subject %s is not permitted for the requested target", p.subject)
}

type targetAuthorizer interface {
	authorizeResolvedTarget(subject string, requested route, instance *types.InstanceSpecification) error
}

type allowAllTargetAuthorizer struct{}

func (allowAllTargetAuthorizer) authorizeResolvedTarget(string, route, *types.InstanceSpecification) error {
	return nil
}

type staticKeyAuthorizer struct {
	principals map[string]keyPrincipal
}

func (a staticKeyAuthorizer) authorizeResolvedTarget(subject string, requested route,
	instance *types.InstanceSpecification) error {
	for _, principal := range a.principals {
		if principal.subject == subject {
			if !principal.allowAll {
				return fmt.Errorf("SSH subject %s is not permitted for instance %s", subject, instance.InstanceID)
			}
			return nil
		}
	}
	return fmt.Errorf("unknown SSH subject %q", subject)
}
