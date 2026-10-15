"""Exports preserve qualification and backend measurement meaning."""
import json
from pathlib import Path
import tempfile
import unittest

from export import measurements


class ExportMeasurements(unittest.TestCase):
    def row(self, *, noop=False, backend='sqlite', throttles=0):
        config = dict(Rate=1000, Seconds=30, Workspaces=1, Outputs=100,
                      Groups=1, BodyBytes=55, RetainPadding=False,
                      Condition='average', Receiver='', Noop=noop, Backend=backend)
        load = dict(offered=30000, accepted=30000-throttles,
                    firstAccepted=30000-throttles, durationSeconds=30,
                    p99MS=195, httpP99MS=190, scheduleP99MS=6,
                    generatorBusy=0, rejected=0, unconfirmed=0,
                    statuses={'202': 30000-throttles, '429': throttles})
        account = dict(Rows=dict(observations=0, entities=0, pending=0, liveBytes=1234),
                       Health=dict(databaseBytes=4096, walBytes=8192))
        report = dict(Config=config, Load=load, Before=[account], After=[account],
                      Resolved=0, Errors=[], ReadCount=0, ReadErrors=0, ReadP99MS=0,
                      Preload={'accepted': 0}, ReceiverUnique=0,
                      DeliveryDrainSeconds=0, Samples=[])
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); case=root/'case';case.mkdir()
            files = {root/'manifest.json': dict(revision='fixture', environment='container', profiles=False),
                     root/'summary.json': [dict(case='case', stable=True)],
                     case/'report.json': report,
                     case/'execution.json': dict(exitCode=0, generatorExitCode=0)}
            for path,value in files.items():path.write_text(json.dumps(value))
            return next(measurements(root))

    def test_generator_only_acknowledgements_are_not_observations(self):
        self.assertEqual(self.row(noop=True)['observationRate'], 0)
        self.assertEqual(self.row()['observationRate'], 100000)

    def test_postgres_logical_usage_is_not_a_sqlite_file_measurement(self):
        row=self.row(backend='postgres')
        self.assertEqual(row['backend'], 'postgres')
        self.assertEqual(row['quotaBytesAfterPostchecks'], 1234)
        self.assertIsNone(row['databaseGrowthBytes'])
        self.assertIsNone(row['afterDatabaseBytes'])
        self.assertIsNone(row['afterWALBytes'])

    def test_historical_pass_with_23_throttles_is_preserved_and_regraded(self):
        row=self.row(throttles=23)
        self.assertTrue(row['reportedStable'])
        self.assertGreaterEqual(row['firstAcceptedFraction'], .999)
        self.assertFalse(row['stable'])
        self.assertTrue(self.row()['stable'])
