/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2026. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

package sshproxy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSHServerConnectionLimit(t *testing.T) {
	srv := &server{slots: make(chan struct{}, 1)}

	require.True(t, srv.acquireConnection())
	require.False(t, srv.acquireConnection())
	srv.releaseConnection()
	require.True(t, srv.acquireConnection())
	srv.releaseConnection()
}
