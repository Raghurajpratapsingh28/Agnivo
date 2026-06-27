package placement_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/scheduler/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/scheduler/placement"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleServers() []model.Server {
	return []model.Server{
		{NodeID: "n1", CPUCores: 4, MemoryMB: 8192, MaxContainers: 10, ContainerCount: 2, HealthStatus: model.HealthHealthy, AdvertiseHost: "10.0.0.1"},
		{NodeID: "n2", CPUCores: 8, MemoryMB: 16384, MaxContainers: 20, ContainerCount: 8, HealthStatus: model.HealthHealthy, AdvertiseHost: "10.0.0.2"},
	}
}

func TestLeastLoadedSelectsServer(t *testing.T) {
	p := &placement.LeastLoadedPlacer{}
	req := model.PlacementRequest{DeploymentID: "d1", CPUMillicores: 250, MemoryMB: 512, PortMin: 30000, PortMax: 40000}
	srv, port, ok := p.Select(sampleServers(), req, 1.0)
	require.True(t, ok)
	assert.Equal(t, "n1", srv.NodeID)
	assert.Greater(t, port, 0)
}

func TestFirstFit(t *testing.T) {
	p := &placement.FirstFitPlacer{}
	req := model.PlacementRequest{DeploymentID: "d2", CPUMillicores: 250, MemoryMB: 512, PortMin: 30000, PortMax: 40000}
	_, _, ok := p.Select(sampleServers(), req, 1.0)
	assert.True(t, ok)
}

func TestGPURequiresGPU(t *testing.T) {
	p := &placement.GPUPlacer{}
	req := model.PlacementRequest{DeploymentID: "d3", CPUMillicores: 250, MemoryMB: 512, PortMin: 30000, PortMax: 40000}
	_, _, ok := p.Select(sampleServers(), req, 1.0)
	assert.False(t, ok)
}

func TestRegistryGet(t *testing.T) {
	reg := placement.NewRegistry(placement.LeastLoaded)
	assert.Equal(t, placement.LeastLoaded, reg.Get(placement.LeastLoaded).Name())
	assert.NotNil(t, reg.Get("unknown"))
}

func TestRegionAwareFilters(t *testing.T) {
	servers := sampleServers()
	servers[0].Region = "us-east-1"
	servers[1].Region = "eu-west-1"
	p := &placement.RegionAwarePlacer{}
	req := model.PlacementRequest{DeploymentID: "d4", Region: "eu-west-1", CPUMillicores: 250, MemoryMB: 512, PortMin: 30000, PortMax: 40000}
	srv, _, ok := p.Select(servers, req, 1.0)
	require.True(t, ok)
	assert.Equal(t, "eu-west-1", srv.Region)
}
