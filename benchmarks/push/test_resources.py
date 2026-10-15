import unittest
from resources import resource_limits


class ResourceProfiles(unittest.TestCase):
    def test_legacy_budgets_remain_comparable(self):
        self.assertEqual(resource_limits('postgres', 2, .5), {
            'appCPUs': 1, 'appMemoryMiB': 1024, 'databaseCPUs': 1, 'databaseMemoryMiB': 1024})
        self.assertEqual(resource_limits('sqlite', 1, .5), {
            'appCPUs': 1, 'appMemoryMiB': 1024, 'databaseCPUs': 0, 'databaseMemoryMiB': 0})

    def test_database_can_grow_without_changing_app(self):
        small = resource_limits('postgres', 2, .5, 1, 1024, 1, 2048)
        large = resource_limits('postgres', 2, .5, 1, 1024, 2, 4096)
        self.assertEqual(small['appCPUs'], large['appCPUs'])
        self.assertEqual(small['appMemoryMiB'], large['appMemoryMiB'])
        self.assertEqual(large['databaseCPUs'], 2)
        self.assertEqual(large['databaseMemoryMiB'], 4096)

    def test_invalid_or_irrelevant_limits_are_rejected(self):
        for kwargs in ({'app_cpus': float('nan')}, {'app_cpus': float('inf')},
                       {'app_cpus': 0}, {'database_memory_mib': 64}):
            with self.assertRaises(ValueError):
                resource_limits('postgres', 2, .5, **kwargs)
        with self.assertRaises(ValueError):
            resource_limits('sqlite', 1, .5, database_cpus=1)
