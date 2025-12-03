/*
 * Copyright (c) 2022 Huawei Technologies Co., Ltd
 *
 * This software is licensed under Mulan PSL v2.
 * You can use this software according to the terms and conditions of the Mulan PSL v2.
 * You may obtain a copy of Mulan PSL v2 at:
 *
 * http://license.coscl.org.cn/MulanPSL2
 *
 * THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY KIND,
 * EITHER EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO NON-INFRINGEMENT,
 * MERCHANTABILITY OR FIT FOR A PARTICULAR PURPOSE.
 * See the Mulan PSL v2 for more details.
 */

// Package metrics -
package metrics

import (
	"yuanrong.org/kernel/pkg/functionscaler/types"
)

// Collector collects metrics data
type Collector interface {
	InvokeMetricsCollected() bool
	UpdateInvokeRequests(insThdReqNum int)
	UpdateInvokeMetrics(insThdMetrics *types.InstanceThreadMetrics)
	UpdateInsThdMetrics(inUseInsThdDiff int)
	GetCalculatedInvokeMetrics() (float64, float64, float64)
	Stop()
}
