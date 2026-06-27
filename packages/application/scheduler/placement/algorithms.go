package placement

import (
	"fmt"
	"math"
	"sort"

	"github.com/agnivo/agnivo/packages/application/scheduler/model"
)

// Algorithm names.
const (
	FirstFit            = "first_fit"
	BestFit             = "best_fit"
	WorstFit            = "worst_fit"
	FirstFitDecreasing  = "first_fit_decreasing"
	BestFitDecreasing   = "best_fit_decreasing"
	LeastLoaded         = "least_loaded"
	MostLoaded          = "most_loaded"
	Balanced            = "balanced"
	Affinity            = "affinity"
	AntiAffinity        = "anti_affinity"
	RegionAware         = "region_aware"
	AZAware             = "az_aware"
	GPU                 = "gpu"
	WarmNode            = "warm_node"
	ColdNode            = "cold_node"
)

// Placer selects a server for a workload.
type Placer interface {
	Name() string
	Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool)
}

// Registry maps algorithm names to placers.
type Registry struct {
	placers map[string]Placer
	defaultPlacer Placer
}

// NewRegistry constructs a placer registry with all algorithms.
func NewRegistry(defaultName string) *Registry {
	base := []Placer{
		&FirstFitPlacer{}, &BestFitPlacer{}, &WorstFitPlacer{},
		&FirstFitDecreasingPlacer{}, &BestFitDecreasingPlacer{},
		&LeastLoadedPlacer{}, &MostLoadedPlacer{}, &BalancedPlacer{},
		&AffinityPlacer{}, &AntiAffinityPlacer{}, &RegionAwarePlacer{},
		&AZAwarePlacer{}, &GPUPlacer{}, &WarmNodePlacer{}, &ColdNodePlacer{},
	}
	m := make(map[string]Placer, len(base))
	for _, p := range base {
		m[p.Name()] = p
	}
	def := m[LeastLoaded]
	if p, ok := m[defaultName]; ok {
		def = p
	}
	return &Registry{placers: m, defaultPlacer: def}
}

// Get returns a placer by name.
func (r *Registry) Get(name string) Placer {
	if p, ok := r.placers[name]; ok {
		return p
	}
	return r.defaultPlacer
}

func fits(s model.Server, req model.PlacementRequest, overcommit float64) bool {
	cpuNeed := float64(req.CPUMillicores) / 1000.0
	if s.AvailableCPU(overcommit) < cpuNeed {
		return false
	}
	if s.AvailableMemoryMB(overcommit) < int64(req.MemoryMB) {
		return false
	}
	if s.AvailableSlots() <= 0 {
		return false
	}
	if req.GPURequired && s.GPUCount <= 0 {
		return false
	}
	return s.HealthStatus == model.HealthHealthy || s.HealthStatus == model.HealthDegraded
}

func pickPort(s model.Server, req model.PlacementRequest, used map[string]bool) int {
	min, max := req.PortMin, req.PortMax
	if min <= 0 {
		min = 30000
	}
	if max <= min {
		max = min + 10000
	}
	// Deterministic port from deployment hash
	base := min + (len(req.DeploymentID) % (max - min + 1))
	for i := 0; i < 100; i++ {
		port := base + i
		if port > max {
			port = min + (port - max)
		}
		key := fmt.Sprintf("%s:%d", s.NodeID, port)
		if !used[key] {
			return port
		}
	}
	return base
}

// --- First Fit ---

type FirstFitPlacer struct{}

func (p *FirstFitPlacer) Name() string { return FirstFit }

func (p *FirstFitPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	used := map[string]bool{}
	for _, s := range servers {
		if !fits(s, req, overcommit) {
			continue
		}
		return s, pickPort(s, req, used), true
	}
	return model.Server{}, 0, false
}

// --- Best Fit ---

type BestFitPlacer struct{}

func (p *BestFitPlacer) Name() string { return BestFit }

func (p *BestFitPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	cpuNeed := float64(req.CPUMillicores) / 1000.0
	var best model.Server
	bestWaste := math.MaxFloat64
	found := false
	used := map[string]bool{}
	for _, s := range servers {
		if !fits(s, req, overcommit) {
			continue
		}
		waste := s.AvailableCPU(overcommit) - cpuNeed
		if waste < bestWaste {
			bestWaste = waste
			best = s
			found = true
		}
	}
	if !found {
		return model.Server{}, 0, false
	}
	return best, pickPort(best, req, used), true
}

// --- Worst Fit ---

type WorstFitPlacer struct{}

func (p *WorstFitPlacer) Name() string { return WorstFit }

func (p *WorstFitPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	cpuNeed := float64(req.CPUMillicores) / 1000.0
	var best model.Server
	bestWaste := -1.0
	found := false
	used := map[string]bool{}
	for _, s := range servers {
		if !fits(s, req, overcommit) {
			continue
		}
		waste := s.AvailableCPU(overcommit) - cpuNeed
		if waste > bestWaste {
			bestWaste = waste
			best = s
			found = true
		}
	}
	if !found {
		return model.Server{}, 0, false
	}
	return best, pickPort(best, req, used), true
}

// --- First Fit Decreasing ---

type FirstFitDecreasingPlacer struct{ base FirstFitPlacer }

func (p *FirstFitDecreasingPlacer) Name() string { return FirstFitDecreasing }

func (p *FirstFitDecreasingPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].AvailableCPU(overcommit) > servers[j].AvailableCPU(overcommit)
	})
	return p.base.Select(servers, req, overcommit)
}

// --- Best Fit Decreasing ---

type BestFitDecreasingPlacer struct{ base BestFitPlacer }

func (p *BestFitDecreasingPlacer) Name() string { return BestFitDecreasing }

func (p *BestFitDecreasingPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].AvailableCPU(overcommit) > servers[j].AvailableCPU(overcommit)
	})
	return p.base.Select(servers, req, overcommit)
}

// --- Least Loaded ---

type LeastLoadedPlacer struct{}

func (p *LeastLoadedPlacer) Name() string { return LeastLoaded }

func (p *LeastLoadedPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	sort.Slice(servers, func(i, j int) bool {
		li := loadScore(servers[i], overcommit)
		lj := loadScore(servers[j], overcommit)
		return li < lj
	})
	return (&FirstFitPlacer{}).Select(servers, req, overcommit)
}

func loadScore(s model.Server, overcommit float64) float64 {
	cpuCap := s.CPUCores * overcommit
	if cpuCap <= 0 {
		return 1
	}
	cpuLoad := s.ReservedCPU / cpuCap
	memCap := float64(s.MemoryMB) * overcommit
	memLoad := 0.0
	if memCap > 0 {
		memLoad = float64(s.ReservedMemoryMB) / memCap
	}
	slotLoad := 0.0
	if s.MaxContainers > 0 {
		slotLoad = float64(s.ContainerCount) / float64(s.MaxContainers)
	}
	return (cpuLoad + memLoad + slotLoad) / 3.0
}

// --- Most Loaded ---

type MostLoadedPlacer struct{ base LeastLoadedPlacer }

func (p *MostLoadedPlacer) Name() string { return MostLoaded }

func (p *MostLoadedPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	sort.Slice(servers, func(i, j int) bool {
		return loadScore(servers[i], overcommit) > loadScore(servers[j], overcommit)
	})
	return (&FirstFitPlacer{}).Select(servers, req, overcommit)
}

// --- Balanced ---

type BalancedPlacer struct{ base LeastLoadedPlacer }

func (p *BalancedPlacer) Name() string { return Balanced }

func (p *BalancedPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	return p.base.Select(servers, req, overcommit)
}

// --- Affinity ---

type AffinityPlacer struct{ base FirstFitPlacer }

func (p *AffinityPlacer) Name() string { return Affinity }

func (p *AffinityPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	if label, ok := req.Labels["affinity"]; ok && label != "" {
		filtered := make([]model.Server, 0)
		for _, s := range servers {
			if s.Region == label || s.NodeID == label {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) > 0 {
			servers = filtered
		}
	}
	return p.base.Select(servers, req, overcommit)
}

// --- Anti-Affinity ---

type AntiAffinityPlacer struct{ base FirstFitPlacer }

func (p *AntiAffinityPlacer) Name() string { return AntiAffinity }

func (p *AntiAffinityPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	if label, ok := req.Labels["anti_affinity"]; ok && label != "" {
		filtered := make([]model.Server, 0)
		for _, s := range servers {
			if s.Region != label && s.NodeID != label {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) > 0 {
			servers = filtered
		}
	}
	return p.base.Select(servers, req, overcommit)
}

// --- Region Aware ---

type RegionAwarePlacer struct{ base FirstFitPlacer }

func (p *RegionAwarePlacer) Name() string { return RegionAware }

func (p *RegionAwarePlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	if req.Region != "" {
		filtered := make([]model.Server, 0)
		for _, s := range servers {
			if s.Region == req.Region {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) > 0 {
			servers = filtered
		}
	}
	return p.base.Select(servers, req, overcommit)
}

// --- AZ Aware ---

type AZAwarePlacer struct{ base RegionAwarePlacer }

func (p *AZAwarePlacer) Name() string { return AZAware }

func (p *AZAwarePlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	if req.AvailabilityZone != "" {
		filtered := make([]model.Server, 0)
		for _, s := range servers {
			if s.AvailabilityZone == req.AvailabilityZone {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) > 0 {
			servers = filtered
		}
	}
	return p.base.Select(servers, req, overcommit)
}

// --- GPU ---

type GPUPlacer struct{ base FirstFitPlacer }

func (p *GPUPlacer) Name() string { return GPU }

func (p *GPUPlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	req.GPURequired = true
	filtered := make([]model.Server, 0)
	for _, s := range servers {
		if s.GPUCount > 0 {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 {
		return model.Server{}, 0, false
	}
	return p.base.Select(filtered, req, overcommit)
}

// --- Warm Node ---

type WarmNodePlacer struct{ base FirstFitPlacer }

func (p *WarmNodePlacer) Name() string { return WarmNode }

func (p *WarmNodePlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].ContainerCount > servers[j].ContainerCount
	})
	return p.base.Select(servers, req, overcommit)
}

// --- Cold Node ---

type ColdNodePlacer struct{ base FirstFitPlacer }

func (p *ColdNodePlacer) Name() string { return ColdNode }

func (p *ColdNodePlacer) Select(servers []model.Server, req model.PlacementRequest, overcommit float64) (model.Server, int, bool) {
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].ContainerCount < servers[j].ContainerCount
	})
	return p.base.Select(servers, req, overcommit)
}
