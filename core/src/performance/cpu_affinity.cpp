#include "performance/optimization.h"
#include <thread>
#include <fstream>
#include <sstream>
#include <algorithm>

#ifdef __linux__
#include <sched.h>
#include <unistd.h>
#include <pthread.h>
#ifdef HAS_NUMA
#include <numa.h>
#endif
#endif

namespace oms::performance {

bool CPUAffinity::set_thread_affinity(int cpu_id) {
#ifdef __linux__
    cpu_set_t cpuset;
    CPU_ZERO(&cpuset);
    CPU_SET(cpu_id, &cpuset);
    
    pthread_t current_thread = pthread_self();
    int result = pthread_setaffinity_np(current_thread, sizeof(cpu_set_t), &cpuset);
    
    if (result != 0) {
        return false;
    }
    
    // Verify the affinity was set correctly
    CPU_ZERO(&cpuset);
    if (pthread_getaffinity_np(current_thread, sizeof(cpu_set_t), &cpuset) == 0) {
        return CPU_ISSET(cpu_id, &cpuset);
    }
#endif
    return false;
}

bool CPUAffinity::set_numa_node(int numa_node) {
#if defined(__linux__) && defined(HAS_NUMA)
    if (numa_available() < 0) {
        return false;
    }
    
    // Get CPUs for this NUMA node
    struct bitmask* cpumask = numa_allocate_cpumask();
    if (!cpumask) {
        return false;
    }
    
    if (numa_node_to_cpus(numa_node, cpumask) < 0) {
        numa_free_cpumask(cpumask);
        return false;
    }
    
    // Find first available CPU in NUMA node
    int cpu_id = -1;
    for (int i = 0; i < numa_num_possible_cpus(); ++i) {
        if (numa_bitmask_isbitset(cpumask, i)) {
            cpu_id = i;
            break;
        }
    }
    
    numa_free_cpumask(cpumask);
    
    if (cpu_id >= 0) {
        // Set thread affinity to this CPU
        if (set_thread_affinity(cpu_id)) {
            // Set memory allocation preference
            numa_set_preferred(numa_node);
            return true;
        }
    }
#endif
    return false;
}

int CPUAffinity::get_numa_node_for_cpu(int cpu_id) {
#if defined(__linux__) && defined(HAS_NUMA)
    if (numa_available() < 0) {
        return -1;
    }
    
    return numa_node_of_cpu(cpu_id);
#else
    return -1;
#endif
}

std::vector<int> CPUAffinity::get_online_cpus() {
    std::vector<int> cpus;
    
#ifdef __linux__
    std::ifstream file("/sys/devices/system/cpu/online");
    if (!file.is_open()) {
        // Fallback to hardware concurrency
        int num_cpus = std::thread::hardware_concurrency();
        for (int i = 0; i < num_cpus; ++i) {
            cpus.push_back(i);
        }
        return cpus;
    }
    
    std::string line;
    if (std::getline(file, line)) {
        std::istringstream iss(line);
        std::string range;
        
        while (std::getline(iss, range, ',')) {
            size_t dash_pos = range.find('-');
            if (dash_pos != std::string::npos) {
                // Handle range like "0-7"
                int start = std::stoi(range.substr(0, dash_pos));
                int end = std::stoi(range.substr(dash_pos + 1));
                for (int i = start; i <= end; ++i) {
                    cpus.push_back(i);
                }
            } else {
                // Single CPU
                cpus.push_back(std::stoi(range));
            }
        }
    }
#else
    // Fallback for non-Linux systems
    int num_cpus = std::thread::hardware_concurrency();
    for (int i = 0; i < num_cpus; ++i) {
        cpus.push_back(i);
    }
#endif
    
    return cpus;
}

int CPUAffinity::get_current_cpu() {
#ifdef __linux__
    return sched_getcpu();
#else
    return -1;
#endif
}

} // namespace oms::performance