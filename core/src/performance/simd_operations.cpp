#include "performance/optimization.h"
#include <immintrin.h>
#include <cpuid.h>
#include <cstring>
#include <algorithm>
#include <limits>

namespace oms::performance {

// CPU feature detection
static bool cpu_has_avx2() {
    unsigned int eax, ebx, ecx, edx;
    if (__get_cpuid_max(0, nullptr) >= 7) {
        __cpuid_count(7, 0, eax, ebx, ecx, edx);
        return (ebx & (1 << 5)) != 0; // AVX2 bit
    }
    return false;
}

static bool cpu_has_avx512() {
    unsigned int eax, ebx, ecx, edx;
    if (__get_cpuid_max(0, nullptr) >= 7) {
        __cpuid_count(7, 0, eax, ebx, ecx, edx);
        return (ebx & (1 << 16)) != 0; // AVX-512F bit
    }
    return false;
}

bool SIMDOps::has_avx2() {
    static bool cached_result = cpu_has_avx2();
    return cached_result;
}

bool SIMDOps::has_avx512() {
    static bool cached_result = cpu_has_avx512();
    return cached_result;
}

// AVX2 implementations for double precision operations

void SIMDOps::add_prices_avx2(const double* a, const double* b, double* result, size_t count) {
    if (!has_avx2()) {
        // Fallback to scalar
        for (size_t i = 0; i < count; ++i) {
            result[i] = a[i] + b[i];
        }
        return;
    }
    
    const size_t simd_width = 4; // 256-bit / 64-bit = 4 doubles
    const size_t simd_count = count / simd_width;
    const size_t remainder = count % simd_width;
    
    // Process aligned data with AVX2
    for (size_t i = 0; i < simd_count; ++i) {
        __m256d va = _mm256_loadu_pd(&a[i * simd_width]);
        __m256d vb = _mm256_loadu_pd(&b[i * simd_width]);
        __m256d vresult = _mm256_add_pd(va, vb);
        _mm256_storeu_pd(&result[i * simd_width], vresult);
    }
    
    // Process remainder
    for (size_t i = simd_count * simd_width; i < count; ++i) {
        result[i] = a[i] + b[i];
    }
}

void SIMDOps::multiply_prices_avx2(const double* prices, double multiplier, double* result, size_t count) {
    if (!has_avx2()) {
        // Fallback to scalar
        for (size_t i = 0; i < count; ++i) {
            result[i] = prices[i] * multiplier;
        }
        return;
    }
    
    const size_t simd_width = 4;
    const size_t simd_count = count / simd_width;
    const size_t remainder = count % simd_width;
    
    // Broadcast multiplier to all lanes
    __m256d vmultiplier = _mm256_set1_pd(multiplier);
    
    // Process aligned data
    for (size_t i = 0; i < simd_count; ++i) {
        __m256d vprices = _mm256_loadu_pd(&prices[i * simd_width]);
        __m256d vresult = _mm256_mul_pd(vprices, vmultiplier);
        _mm256_storeu_pd(&result[i * simd_width], vresult);
    }
    
    // Process remainder
    for (size_t i = simd_count * simd_width; i < count; ++i) {
        result[i] = prices[i] * multiplier;
    }
}

double SIMDOps::sum_prices_avx2(const double* prices, size_t count) {
    if (!has_avx2() || count < 4) {
        // Fallback to scalar
        double sum = 0.0;
        for (size_t i = 0; i < count; ++i) {
            sum += prices[i];
        }
        return sum;
    }
    
    const size_t simd_width = 4;
    const size_t simd_count = count / simd_width;
    const size_t remainder = count % simd_width;
    
    // Initialize accumulator
    __m256d vsum = _mm256_setzero_pd();
    
    // Main loop - unroll by 4 for better performance
    size_t i = 0;
    for (; i + 4 <= simd_count; i += 4) {
        __m256d v0 = _mm256_loadu_pd(&prices[(i + 0) * simd_width]);
        __m256d v1 = _mm256_loadu_pd(&prices[(i + 1) * simd_width]);
        __m256d v2 = _mm256_loadu_pd(&prices[(i + 2) * simd_width]);
        __m256d v3 = _mm256_loadu_pd(&prices[(i + 3) * simd_width]);
        
        vsum = _mm256_add_pd(vsum, v0);
        vsum = _mm256_add_pd(vsum, v1);
        vsum = _mm256_add_pd(vsum, v2);
        vsum = _mm256_add_pd(vsum, v3);
    }
    
    // Process remaining full vectors
    for (; i < simd_count; ++i) {
        __m256d v = _mm256_loadu_pd(&prices[i * simd_width]);
        vsum = _mm256_add_pd(vsum, v);
    }
    
    // Horizontal sum of vector
    __m128d vlow = _mm256_castpd256_pd128(vsum);
    __m128d vhigh = _mm256_extractf128_pd(vsum, 1);
    __m128d vsum128 = _mm_add_pd(vlow, vhigh);
    __m128d vshuf = _mm_permute_pd(vsum128, 0x1);
    __m128d vresult = _mm_add_sd(vsum128, vshuf);
    
    double sum = _mm_cvtsd_f64(vresult);
    
    // Add remainder
    for (size_t j = simd_count * simd_width; j < count; ++j) {
        sum += prices[j];
    }
    
    return sum;
}

void SIMDOps::min_max_prices_avx2(const double* prices, size_t count, double& min_val, double& max_val) {
    if (!has_avx2() || count == 0) {
        // Fallback to scalar
        min_val = std::numeric_limits<double>::max();
        max_val = std::numeric_limits<double>::lowest();
        for (size_t i = 0; i < count; ++i) {
            min_val = std::min(min_val, prices[i]);
            max_val = std::max(max_val, prices[i]);
        }
        return;
    }
    
    const size_t simd_width = 4;
    const size_t simd_count = count / simd_width;
    const size_t remainder = count % simd_width;
    
    // Initialize with first element
    __m256d vmin = _mm256_set1_pd(prices[0]);
    __m256d vmax = _mm256_set1_pd(prices[0]);
    
    // Process vectors
    for (size_t i = 0; i < simd_count; ++i) {
        __m256d v = _mm256_loadu_pd(&prices[i * simd_width]);
        vmin = _mm256_min_pd(vmin, v);
        vmax = _mm256_max_pd(vmax, v);
    }
    
    // Extract min/max from vectors
    alignas(32) double min_arr[4];
    alignas(32) double max_arr[4];
    _mm256_store_pd(min_arr, vmin);
    _mm256_store_pd(max_arr, vmax);
    
    min_val = min_arr[0];
    max_val = max_arr[0];
    for (int i = 1; i < 4; ++i) {
        min_val = std::min(min_val, min_arr[i]);
        max_val = std::max(max_val, max_arr[i]);
    }
    
    // Process remainder
    for (size_t i = simd_count * simd_width; i < count; ++i) {
        min_val = std::min(min_val, prices[i]);
        max_val = std::max(max_val, prices[i]);
    }
}

void SIMDOps::compare_prices_avx2(const double* a, const double* b, bool* result, size_t count) {
    if (!has_avx2()) {
        // Fallback to scalar
        for (size_t i = 0; i < count; ++i) {
            result[i] = (a[i] > b[i]);
        }
        return;
    }
    
    const size_t simd_width = 4;
    const size_t simd_count = count / simd_width;
    const size_t remainder = count % simd_width;
    
    // Process vectors
    for (size_t i = 0; i < simd_count; ++i) {
        __m256d va = _mm256_loadu_pd(&a[i * simd_width]);
        __m256d vb = _mm256_loadu_pd(&b[i * simd_width]);
        
        // Compare for greater than
        __m256d vcmp = _mm256_cmp_pd(va, vb, _CMP_GT_OQ);
        
        // Extract mask and convert to bool array
        int mask = _mm256_movemask_pd(vcmp);
        for (size_t j = 0; j < simd_width; ++j) {
            result[i * simd_width + j] = (mask & (1 << j)) != 0;
        }
    }
    
    // Process remainder
    for (size_t i = simd_count * simd_width; i < count; ++i) {
        result[i] = (a[i] > b[i]);
    }
}

// Additional SIMD utilities for order processing

// Fast memory copy optimized for small fixed-size structures
template<size_t Size>
void simd_memcpy_small(void* dst, const void* src) {
    if constexpr (Size == 16) {
        __m128i data = _mm_loadu_si128(reinterpret_cast<const __m128i*>(src));
        _mm_storeu_si128(reinterpret_cast<__m128i*>(dst), data);
    } else if constexpr (Size == 32) {
        __m256i data = _mm256_loadu_si256(reinterpret_cast<const __m256i*>(src));
        _mm256_storeu_si256(reinterpret_cast<__m256i*>(dst), data);
    } else if constexpr (Size == 64) {
        __m256i data0 = _mm256_loadu_si256(reinterpret_cast<const __m256i*>(src));
        __m256i data1 = _mm256_loadu_si256(reinterpret_cast<const __m256i*>(
            static_cast<const char*>(src) + 32));
        _mm256_storeu_si256(reinterpret_cast<__m256i*>(dst), data0);
        _mm256_storeu_si256(reinterpret_cast<__m256i*>(
            static_cast<char*>(dst) + 32), data1);
    } else {
        std::memcpy(dst, src, Size);
    }
}

// Template instantiations
template void simd_memcpy_small<16>(void*, const void*);
template void simd_memcpy_small<32>(void*, const void*);
template void simd_memcpy_small<64>(void*, const void*);

// Fast hash computation using SIMD
uint64_t simd_hash_64(const void* data, size_t len) {
    const uint64_t prime = 0x00000100000001B3ULL;
    const uint64_t seed = 0x9E3779B97F4A7C15ULL;
    
    uint64_t hash = seed;
    const uint64_t* ptr = reinterpret_cast<const uint64_t*>(data);
    const size_t qwords = len / 8;
    
    // Process 8-byte chunks
    for (size_t i = 0; i < qwords; ++i) {
        hash ^= ptr[i];
        hash *= prime;
        hash ^= (hash >> 33);
    }
    
    // Process remaining bytes
    const uint8_t* bytes = reinterpret_cast<const uint8_t*>(data) + qwords * 8;
    for (size_t i = qwords * 8; i < len; ++i) {
        hash ^= static_cast<uint64_t>(bytes[i - qwords * 8]) << ((i & 7) * 8);
    }
    
    // Final mixing
    hash ^= hash >> 33;
    hash *= prime;
    hash ^= hash >> 33;
    
    return hash;
}

} // namespace oms::performance