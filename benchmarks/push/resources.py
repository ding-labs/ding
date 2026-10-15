"""Explicit app/database allocations, independent of the legacy equal split."""
import math


def resource_limits(backend, units, share, app_cpus=None, app_memory_mib=None,
                    database_cpus=None, database_memory_mib=None):
    postgres = backend == 'postgres'
    if not postgres and (database_cpus is not None or database_memory_mib is not None):
        raise ValueError('database resource overrides require PostgreSQL')
    result = {
        'appCPUs': units*(1-share) if postgres else units,
        'appMemoryMiB': units*512 if postgres else units*1024,
        'databaseCPUs': units*share if postgres else 0,
        'databaseMemoryMiB': units*512 if postgres else 0,
    }
    for key, override in [('appCPUs', app_cpus), ('appMemoryMiB', app_memory_mib),
                          ('databaseCPUs', database_cpus), ('databaseMemoryMiB', database_memory_mib)]:
        if override is not None:
            result[key] = override
    for prefix in ['app'] + (['database'] if postgres else []):
        cpus, memory = result[prefix+'CPUs'], result[prefix+'MemoryMiB']
        if not math.isfinite(cpus) or not .1 <= cpus <= 64:
            raise ValueError('CPU allocation must be finite and between 0.1 and 64')
        if not isinstance(memory, int) or not 128 <= memory <= 65536:
            raise ValueError('memory allocation must be 128..65536 MiB')
    return result
