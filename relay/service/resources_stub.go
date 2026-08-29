//go:build !linux

package service

import "relay/model"

func QueryResourceMetrics() (model.ResourceMetrics, error) {
	return model.ResourceMetrics{}, errResourcesUnsupported
}
