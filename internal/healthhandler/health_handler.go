/*
Copyright 2022 Nokia.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package healthhandler

import (
	"context"

	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Check implements `service Health`.
func (s *subServer) Check(ctx context.Context, in *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	s.l = log.FromContext(ctx)
	s.l.Info("grpc server health check", "service", in.Service)
	s.mu.RLock()
	defer s.mu.RUnlock()
	//if servingStatus, ok := s.statusMap[in.Service]; ok {
	if servingStatus, ok := s.statusMap[""]; ok {
		return &healthpb.HealthCheckResponse{
			Status: servingStatus,
		}, nil
	}
	return nil, status.Error(codes.NotFound, "unknown service")
}

// Watch implements `service Health`.
func (s *subServer) Watch(in *healthpb.HealthCheckRequest, stream healthpb.Health_WatchServer) error {
	s.l.Info("grpc server health watch", "service", in.Service)

	//service := in.Service
	service := ""
	// update channel is used for getting service status updates.
	update := make(chan healthpb.HealthCheckResponse_ServingStatus, 1)
	s.mu.Lock()
	// Puts the initial status to the channel.
	if servingStatus, ok := s.statusMap[service]; ok {
		update <- servingStatus
	} else {
		update <- healthpb.HealthCheckResponse_SERVICE_UNKNOWN
	}

	// Registers the update channel to the correct place in the updates map.
	if _, ok := s.updates[service]; !ok {
		s.updates[service] = make(map[healthpb.Health_WatchServer]chan healthpb.HealthCheckResponse_ServingStatus)
	}
	s.updates[service][stream] = update
	defer func() {
		s.mu.Lock()
		delete(s.updates[service], stream)
		s.mu.Unlock()
	}()
	s.mu.Unlock()

	var lastSentStatus healthpb.HealthCheckResponse_ServingStatus = -1
	for {
		select {
		// Status updated. Sends the up-to-date status to the client.
		case servingStatus := <-update:
			if lastSentStatus == servingStatus {
				continue
			}
			lastSentStatus = servingStatus
			err := stream.Send(&healthpb.HealthCheckResponse{Status: servingStatus})
			if err != nil {
				return status.Error(codes.Canceled, "Stream has ended.")
			}
		// Context done. Removes the update channel from the updates map.
		case <-stream.Context().Done():
			return status.Error(codes.Canceled, "Stream has ended.")
		}
	}
}
