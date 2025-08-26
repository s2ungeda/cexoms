#ifndef OMS_PERFORMANCE_PROFILE_H
#define OMS_PERFORMANCE_PROFILE_H

#include <chrono>
#include <string>
#include <vector>
#include <unordered_map>
#include <atomic>
#include <mutex>
#include <cstdint>
#include <algorithm>
#include <numeric>
#include <cmath>
#include <thread>
#include <x86intrin.h>

namespace oms::performance {

// High-resolution timer using TSC (Time Stamp Counter)
class TSCTimer {
private:
    uint64_t start_tsc_;
    
public:
    TSCTimer() : start_tsc_(__rdtsc()) {}
    
    void reset() {
        start_tsc_ = __rdtsc();
    }
    
    uint64_t elapsed_cycles() const {
        return __rdtsc() - start_tsc_;
    }
    
    // Convert TSC cycles to nanoseconds (requires calibration)
    double elapsed_ns() const {
        static double tsc_freq = calibrate_tsc();
        return elapsed_cycles() / tsc_freq;
    }
    
private:
    static double calibrate_tsc() {
        // Calibrate TSC frequency
        auto start_time = std::chrono::high_resolution_clock::now();
        uint64_t start_tsc = __rdtsc();
        
        // Sleep for calibration period
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
        
        auto end_time = std::chrono::high_resolution_clock::now();
        uint64_t end_tsc = __rdtsc();
        
        auto duration_ns = std::chrono::duration_cast<std::chrono::nanoseconds>(
            end_time - start_time).count();
        
        return static_cast<double>(end_tsc - start_tsc) / duration_ns;
    }
};

// Profiling statistics
struct ProfileStats {
    uint64_t count = 0;
    double min_ns = std::numeric_limits<double>::max();
    double max_ns = 0.0;
    double total_ns = 0.0;
    double sum_squares = 0.0;
    
    void record(double elapsed_ns) {
        count++;
        min_ns = std::min(min_ns, elapsed_ns);
        max_ns = std::max(max_ns, elapsed_ns);
        total_ns += elapsed_ns;
        sum_squares += elapsed_ns * elapsed_ns;
    }
    
    double mean() const {
        return count > 0 ? total_ns / count : 0.0;
    }
    
    double stddev() const {
        if (count <= 1) return 0.0;
        double mean_val = mean();
        double variance = (sum_squares / count) - (mean_val * mean_val);
        return std::sqrt(std::max(0.0, variance));
    }
    
    double percentile(const std::vector<double>& sorted_samples, double p) const {
        if (sorted_samples.empty()) return 0.0;
        size_t idx = static_cast<size_t>(sorted_samples.size() * p / 100.0);
        return sorted_samples[std::min(idx, sorted_samples.size() - 1)];
    }
};

// Thread-safe profiler
class Profiler {
private:
    struct ProfileData {
        ProfileStats stats;
        std::vector<double> samples;
        std::mutex mutex;
    };
    
    std::unordered_map<std::string, ProfileData> profiles_;
    std::mutex map_mutex_;
    bool enabled_ = true;
    
public:
    static Profiler& instance() {
        static Profiler profiler;
        return profiler;
    }
    
    void enable() { enabled_ = true; }
    void disable() { enabled_ = false; }
    bool is_enabled() const { return enabled_; }
    
    void record(const std::string& name, double elapsed_ns) {
        if (!enabled_) return;
        
        ProfileData* data = nullptr;
        {
            std::lock_guard<std::mutex> lock(map_mutex_);
            data = &profiles_[name];
        }
        
        std::lock_guard<std::mutex> lock(data->mutex);
        data->stats.record(elapsed_ns);
        
        // Keep detailed samples for percentile calculation (limit to 10k samples)
        if (data->samples.size() < 10000) {
            data->samples.push_back(elapsed_ns);
        }
    }
    
    ProfileStats get_stats(const std::string& name) {
        ProfileData* data = nullptr;
        {
            std::lock_guard<std::mutex> lock(map_mutex_);
            auto it = profiles_.find(name);
            if (it == profiles_.end()) {
                return ProfileStats{};
            }
            data = &it->second;
        }
        
        std::lock_guard<std::mutex> lock(data->mutex);
        ProfileStats stats = data->stats;
        
        // Calculate percentiles if we have samples
        if (!data->samples.empty()) {
            std::vector<double> sorted_samples = data->samples;
            std::sort(sorted_samples.begin(), sorted_samples.end());
            
            // Add percentile data to stats (would need to extend ProfileStats struct)
        }
        
        return stats;
    }
    
    void reset() {
        std::lock_guard<std::mutex> lock(map_mutex_);
        profiles_.clear();
    }
    
    std::vector<std::string> get_profile_names() {
        std::lock_guard<std::mutex> lock(map_mutex_);
        std::vector<std::string> names;
        names.reserve(profiles_.size());
        for (const auto& [name, _] : profiles_) {
            names.push_back(name);
        }
        return names;
    }
};

// RAII profiler scope
class ScopedProfiler {
private:
    std::string name_;
    TSCTimer timer_;
    
public:
    explicit ScopedProfiler(const std::string& name) : name_(name) {}
    
    ~ScopedProfiler() {
        if (Profiler::instance().is_enabled()) {
            Profiler::instance().record(name_, timer_.elapsed_ns());
        }
    }
};

// Macros for easy profiling
#define PROFILE_SCOPE(name) \
    oms::performance::ScopedProfiler _profiler_##__LINE__(name)

#define PROFILE_FUNCTION() \
    PROFILE_SCOPE(__FUNCTION__)

// Memory profiling utilities
class MemoryProfiler {
private:
    struct AllocationInfo {
        size_t size;
        void* ptr;
        std::string location;
        std::chrono::steady_clock::time_point timestamp;
    };
    
    std::unordered_map<void*, AllocationInfo> allocations_;
    std::mutex mutex_;
    std::atomic<size_t> total_allocated_{0};
    std::atomic<size_t> peak_allocated_{0};
    std::atomic<size_t> allocation_count_{0};
    bool enabled_ = false;
    
public:
    static MemoryProfiler& instance() {
        static MemoryProfiler profiler;
        return profiler;
    }
    
    void enable() { enabled_ = true; }
    void disable() { enabled_ = false; }
    
    void record_allocation(void* ptr, size_t size, const std::string& location = "") {
        if (!enabled_ || !ptr) return;
        
        {
            std::lock_guard<std::mutex> lock(mutex_);
            allocations_[ptr] = {size, ptr, location, std::chrono::steady_clock::now()};
        }
        
        total_allocated_ += size;
        allocation_count_++;
        
        size_t current = total_allocated_.load();
        size_t peak = peak_allocated_.load();
        while (current > peak && !peak_allocated_.compare_exchange_weak(peak, current)) {
            // Update peak
        }
    }
    
    void record_deallocation(void* ptr) {
        if (!enabled_ || !ptr) return;
        
        size_t size = 0;
        {
            std::lock_guard<std::mutex> lock(mutex_);
            auto it = allocations_.find(ptr);
            if (it != allocations_.end()) {
                size = it->second.size;
                allocations_.erase(it);
            }
        }
        
        if (size > 0) {
            total_allocated_ -= size;
        }
    }
    
    size_t get_current_allocated() const {
        return total_allocated_.load();
    }
    
    size_t get_peak_allocated() const {
        return peak_allocated_.load();
    }
    
    size_t get_allocation_count() const {
        return allocation_count_.load();
    }
    
    std::vector<AllocationInfo> get_live_allocations() {
        std::lock_guard<std::mutex> lock(mutex_);
        std::vector<AllocationInfo> result;
        result.reserve(allocations_.size());
        for (const auto& [_, info] : allocations_) {
            result.push_back(info);
        }
        return result;
    }
};

// Cache performance monitoring
class CacheProfiler {
private:
    struct CacheStats {
        std::atomic<uint64_t> l1_hits{0};
        std::atomic<uint64_t> l1_misses{0};
        std::atomic<uint64_t> l2_hits{0};
        std::atomic<uint64_t> l2_misses{0};
        std::atomic<uint64_t> l3_hits{0};
        std::atomic<uint64_t> l3_misses{0};
        std::atomic<uint64_t> tlb_misses{0};
    };
    
    CacheStats stats_;
    
public:
    static CacheProfiler& instance() {
        static CacheProfiler profiler;
        return profiler;
    }
    
    // These would typically use performance counters (perf_event_open on Linux)
    // For now, we provide the interface
    void record_l1_hit() { stats_.l1_hits++; }
    void record_l1_miss() { stats_.l1_misses++; }
    void record_l2_hit() { stats_.l2_hits++; }
    void record_l2_miss() { stats_.l2_misses++; }
    void record_l3_hit() { stats_.l3_hits++; }
    void record_l3_miss() { stats_.l3_misses++; }
    void record_tlb_miss() { stats_.tlb_misses++; }
    
    double get_l1_hit_rate() const {
        uint64_t hits = stats_.l1_hits.load();
        uint64_t misses = stats_.l1_misses.load();
        uint64_t total = hits + misses;
        return total > 0 ? static_cast<double>(hits) / total : 0.0;
    }
    
    double get_l2_hit_rate() const {
        uint64_t hits = stats_.l2_hits.load();
        uint64_t misses = stats_.l2_misses.load();
        uint64_t total = hits + misses;
        return total > 0 ? static_cast<double>(hits) / total : 0.0;
    }
    
    double get_l3_hit_rate() const {
        uint64_t hits = stats_.l3_hits.load();
        uint64_t misses = stats_.l3_misses.load();
        uint64_t total = hits + misses;
        return total > 0 ? static_cast<double>(hits) / total : 0.0;
    }
    
    uint64_t get_tlb_misses() const {
        return stats_.tlb_misses.load();
    }
    
    void reset() {
        stats_.l1_hits = 0;
        stats_.l1_misses = 0;
        stats_.l2_hits = 0;
        stats_.l2_misses = 0;
        stats_.l3_hits = 0;
        stats_.l3_misses = 0;
        stats_.tlb_misses = 0;
    }
};

// Benchmark utilities
template<typename Func>
double benchmark(Func&& func, size_t iterations = 1000000) {
    // Warm up
    for (size_t i = 0; i < std::min(iterations / 10, size_t(1000)); ++i) {
        func();
    }
    
    // Actual benchmark
    TSCTimer timer;
    for (size_t i = 0; i < iterations; ++i) {
        func();
    }
    
    return timer.elapsed_ns() / iterations;
}

template<typename Func>
ProfileStats benchmark_detailed(Func&& func, size_t iterations = 10000) {
    ProfileStats stats;
    std::vector<double> samples;
    samples.reserve(iterations);
    
    // Warm up
    for (size_t i = 0; i < std::min(iterations / 10, size_t(100)); ++i) {
        func();
    }
    
    // Collect samples
    for (size_t i = 0; i < iterations; ++i) {
        TSCTimer timer;
        func();
        double elapsed = timer.elapsed_ns();
        stats.record(elapsed);
        samples.push_back(elapsed);
    }
    
    return stats;
}

} // namespace oms::performance

#endif // OMS_PERFORMANCE_PROFILE_H