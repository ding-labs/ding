import json
import unittest
from postgres_profile import summarize


class SQLProfiles(unittest.TestCase):
    def test_profiles_keep_only_bounded_labels_and_numeric_totals(self):
        rows = [{'query': "SELECT * FROM entities WHERE key='private-input'", 'calls': 3,
                 'total_exec_time': 12, 'unknown': 'secret'},
                {'query': 'SELECT key FROM entities WHERE workspace_id=$1', 'calls': 2,
                 'total_exec_time': 4},
                {'query': "CREATE ROLE user PASSWORD 'secret'", 'calls': 1}]
        result = summarize(rows)
        self.assertEqual(result[0]['operation'], 'select_entities')
        self.assertEqual(result[0]['calls'], 5)
        self.assertEqual(result[0]['total_exec_time'], 16)
        self.assertEqual(result[1]['operation'], 'other')
        self.assertNotIn('secret', json.dumps(result))
        self.assertNotIn('private-input', json.dumps(result))

    def test_ownership_and_scope_have_specific_labels(self):
        self.assertEqual(summarize([{'query': 'SELECT * FROM ding_meta.owner'}])[0]['operation'], 'ownership')
        self.assertEqual(summarize([{'query': "SELECT set_config('ding.workspace',$1,true)"}])[0]['operation'], 'scope')
