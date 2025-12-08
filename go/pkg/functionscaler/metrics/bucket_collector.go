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


// Package metrics -
package metrics

import (
	"math"
	"sync"
	"time"

	"yuanrong.org/kernel/pkg/common/faas_common/logger/log"
	"yuanrong.org/kernel/pkg/common/faas_common/utils"
	"yuanrong.org/kernel/pkg/functionscaler/types"
)

const (
	second2Millisecond      = 1000
	defaultBucketBufferSize = 100
	defaultBucketRotatePace = 1
	factor                  = 0.7
)

// Bucket collects metrics over a fixed time window
type Bucket struct {
	// number of in-used instance threads
	inUseInsThdNum float64
	// max number of in-used instance threads since started
	maxInUseInsThdNum float64
	// number of instance thread request
	insThdReqNum       float64
	prevInsThdReqNumPS float64
	// average process time of all instance threads
	avgProcTime     float64
	prevAvgProcTime float64
	// number of requests processed by one instance thread per second, this is a average value
	insThdProcNumPS     float64
	prevInsThdProcNumPS float64
	initialized         bool
	isFirst             bool
	recordTime          time.Time
}

// BucketCollector collects metrics into buckets periodically
type BucketCollector struct {
	lastValidBucket *Bucket
	bucketBuffer    []*Bucket
	funcKey         string
	resKey          string
	rotatePace      int
	curIndex        int
	started         bool
	collected       bool
	stopCh          chan struct{}
	sync.RWMutex
}

// NewBucketMetricsCollector creates a BucketCollector
func NewBucketMetricsCollector(funcKey, resKey string) Collector {
	return &BucketCollector{
		funcKey:      funcKey,
		resKey:       resKey,
		bucketBuffer: make([]*Bucket, defaultBucketBufferSize, defaultBucketBufferSize),
		curIndex:     0,
		rotatePace:   defaultBucketRotatePace,
		started:      false,
		stopCh:       make(chan struct{}),
	}
}

// InvokeMetricsCollected checks if invoke metrics is collected
func (bm *BucketCollector) InvokeMetricsCollected() bool {
	bm.RLock()
	collected := bm.collected
	bm.RUnlock()
	return collected
}

// UpdateInvokeRequests updates invoke request number
func (bm *BucketCollector) UpdateInvokeRequests(insThdReqNum int) {
	bm.Lock()
	if !bm.started {
		bm.started = true
		bm.bucketBuffer[bm.curIndex] = &Bucket{isFirst: true, recordTime: time.Now()}
		go bm.StartRotateBucket()
	}
	curBucket := bm.bucketBuffer[bm.curIndex]
	curBucket.insThdReqNum += float64(insThdReqNum)
	bm.Unlock()
}

// UpdateInvokeMetrics updates invoke metrics
func (bm *BucketCollector) UpdateInvokeMetrics(insThdMetrics *types.InstanceThreadMetrics) {
	bm.Lock()
	defer bm.Unlock()
	if !bm.started {
		bm.started = true
		bm.bucketBuffer[bm.curIndex] = &Bucket{isFirst: true, recordTime: time.Now()}
		go bm.StartRotateBucket()
	}
	curBucket := bm.bucketBuffer[bm.curIndex]
	// if ProcReqNum == 0 then consider this insThdMetrics is meaningless
	if insThdMetrics != nil && insThdMetrics.ProcReqNum != 0 {
		if insThdMetrics.AvgProcTime == 0 {
			log.GetLogger().Errorf("invalid value in metrics of instance thread %s metrics value %+v",
				insThdMetrics.InsThdID,
				insThdMetrics)
			return
		}
		curThdProcNumPS := second2Millisecond / float64(insThdMetrics.AvgProcTime)
		if !bm.collected {
			bm.collected = true
		}
		if !curBucket.initialized {
			curBucket.initialized = true
			curBucket.avgProcTime = float64(insThdMetrics.AvgProcTime)
			curBucket.insThdProcNumPS = curThdProcNumPS
		} else {
			// average with AvgProcTime in metrics for one instance thread then average with other instance threads
			// to get general avgProcTime
			curBucket.avgProcTime = ((curBucket.avgProcTime+float64(insThdMetrics.AvgProcTime))/2 +
				curBucket.avgProcTime*(curBucket.maxInUseInsThdNum-1)) / curBucket.maxInUseInsThdNum
			// average with curThdPocNumPS calculated from metrics for one instance thread then average with other
			// instance threads to get general insThdProcNumPS
			curBucket.insThdProcNumPS = ((curBucket.insThdProcNumPS+curThdProcNumPS)/2 + curBucket.insThdProcNumPS*
				(curBucket.maxInUseInsThdNum-1)) / curBucket.maxInUseInsThdNum
		}
	}
}

// UpdateInsThdMetrics updates instance thread metrics
func (bm *BucketCollector) UpdateInsThdMetrics(inUseInsThdDiff int) {
	if inUseInsThdDiff == 0 {
		return
	}
	bm.Lock()
	if !bm.started {
		bm.started = true
		bm.bucketBuffer[bm.curIndex] = &Bucket{recordTime: time.Now()}
		go bm.StartRotateBucket()
	}
	curBucket := bm.bucketBuffer[bm.curIndex]
	curBucket.inUseInsThdNum += float64(inUseInsThdDiff)
	curBucket.maxInUseInsThdNum = math.Max(curBucket.inUseInsThdNum, curBucket.maxInUseInsThdNum)
	bm.Unlock()
}

// GetCalculatedInvokeMetrics will get three parameters used by autoScaler:
// 1. average process time of instance thread
// 2. number of requests processed by one instance thread per second
// 3. number of instance thread requests per second
func (bm *BucketCollector) GetCalculatedInvokeMetrics() (float64, float64, float64) {
	bm.RLock()
	curBucket := bm.bucketBuffer[bm.curIndex]
	avgProcTime := averageWithPrev(curBucket.avgProcTime, curBucket.prevAvgProcTime)
	insThdProcNumPS := averageWithPrev(curBucket.insThdProcNumPS, curBucket.prevInsThdProcNumPS)
	// unlike avgProcTime and insThdProcNum, 0 is meaningful for insThdReqNum. calculate the value of insThdReqNumPS
	// by the granularity of second
	curInsThdReqNumPS := curBucket.insThdReqNum / math.Max(math.Ceil(time.Now().Sub(curBucket.recordTime).Seconds()), 1)
	insThdReqNumPS := factor*curInsThdReqNumPS + (1-factor)*curBucket.prevInsThdReqNumPS
	bm.RUnlock()
	return avgProcTime, insThdProcNumPS, insThdReqNumPS
}

// Stop will stop metrics collector, may be called multiple times
func (bm *BucketCollector) Stop() {
	utils.SafeCloseChannel(bm.stopCh)
}

// StartRotateBucket rotates buckets by rotatePace
func (bm *BucketCollector) StartRotateBucket() {
	ticker := time.NewTicker(time.Duration(bm.rotatePace) * time.Second)
	for {
		select {
		case <-ticker.C:
			bm.Lock()
			prevBucket := bm.bucketBuffer[bm.curIndex]
			bm.curIndex = (bm.curIndex + 1) % len(bm.bucketBuffer)
			bm.bucketBuffer[bm.curIndex] = bm.rotateMetricsBucket(prevBucket)
			bm.Unlock()
		case <-bm.stopCh:
			log.GetLogger().Warnf("stop collect metrics of function %s resource %s", bm.funcKey, bm.resKey)
			return
		}
	}
}

func (bm *BucketCollector) rotateMetricsBucket(prevBucket *Bucket) *Bucket {
	prevAvgProcTime := averageWithPrev(prevBucket.avgProcTime, prevBucket.prevAvgProcTime)
	prevInsThdProcNumPS := averageWithPrev(prevBucket.insThdProcNumPS, prevBucket.prevInsThdProcNumPS)
	// unlike avgProcTime and insThdProcNum, 0 is meaningful for insThdReqNum
	var prevInsThdReqNumPS float64
	insThdReqNumPS := prevBucket.insThdReqNum / float64(bm.rotatePace)
	// if preBucket is the init bucket then prevInsThdReqNumPS should not be averaged with its prev
	if prevBucket.isFirst {
		prevInsThdReqNumPS = insThdReqNumPS
	} else {
		prevInsThdReqNumPS = factor*insThdReqNumPS + (1-factor)*prevBucket.prevInsThdReqNumPS
	}
	return &Bucket{
		inUseInsThdNum:      prevBucket.inUseInsThdNum,
		maxInUseInsThdNum:   prevBucket.maxInUseInsThdNum,
		prevAvgProcTime:     prevAvgProcTime,
		prevInsThdProcNumPS: prevInsThdProcNumPS,
		prevInsThdReqNumPS:  prevInsThdReqNumPS,
		recordTime:          time.Now(),
	}
}

// avoid to average with 0 value for parameters such as avgProcTime and insThdProcNumPS, an assumption is made that
// value and preValue won't both be 0
func averageWithPrev(value, prevValue float64) float64 {
	avgValue := float64(0)
	if value != 0 && prevValue != 0 {
		avgValue = factor*value + (1-factor)*prevValue
	} else if value == 0 {
		avgValue = prevValue
	} else if prevValue == 0 {
		avgValue = value
	}
	return avgValue
}
