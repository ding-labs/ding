import unittest
from urllib.parse import urlsplit, parse_qs
from railway_db import connection_url


class RailwayConnections(unittest.TestCase):
    def test_rejects_public_and_overridden_hosts(self):
        for source in ('postgres://u:p@public.example/railway', 'https://postgres.railway.internal/railway'):
            with self.assertRaises(ValueError):
                connection_url(source, '/ca.crt')
        url = urlsplit(connection_url('postgres://u:p@postgres.railway.internal/railway?host=evil.example&sslmode=disable', '/ca.crt'))
        self.assertEqual(parse_qs(url.query), {'sslmode':['verify-full'], 'sslrootcert':['/ca.crt']})

    def test_role_credentials_are_encoded(self):
        url = urlsplit(connection_url('postgres://u:p@postgres.railway.internal/railway', '/ca.crt', 'ding_app', 'a@b:/'))
        self.assertEqual(url.hostname, 'postgres.railway.internal')
        self.assertEqual(url.username, 'ding_app')
        self.assertEqual(url.path, '/railway')
