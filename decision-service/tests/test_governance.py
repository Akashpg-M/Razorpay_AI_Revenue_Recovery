from pathlib import Path
import tempfile
import unittest
from governance.calibration import select_on_validation, calibration_report
from governance.datasets import build_versioned_dataset
from governance.registry import transition

class GovernanceTests(unittest.TestCase):
    def test_dataset_is_immutable_and_excludes_non_production(self):
        rows=[{"id":str(i),"created_at":f"2026-01-{i+1:02d}T00:00:00Z","training_eligible":True,"environment":"production"} for i in range(10)]
        rows.append({"id":"x","created_at":"2026-02-01T00:00:00Z","training_eligible":True,"environment":"simulation"})
        with tempfile.TemporaryDirectory() as directory:
            target=Path(directory);manifest=build_versioned_dataset(rows,target,"feedback-v1","features-v1","label-v1")
            self.assertEqual(manifest.row_count,10);self.assertEqual(manifest.exclusions,{"NON_PRODUCTION":1})
            with self.assertRaises(FileExistsError): build_versioned_dataset(rows,target,"feedback-v1","features-v1","label-v1")

    def test_calibration_uses_validation_and_reports_bins(self):
        chosen,report=select_on_validation([.05,.2,.35,.7,.8,.95],[0,0,1,1,1,1])
        evaluated=calibration_report(chosen.predict([.1,.9]).tolist(),[0,1],["A","B"])
        self.assertEqual(report["selection_split"],"validation");self.assertTrue(evaluated["bins"]);self.assertEqual(set(evaluated["segments"]),{"A","B"})

    def test_registry_requires_explicit_valid_transition(self):
        self.assertEqual(transition("CANDIDATE","APPROVED",actor="reviewer",reason="metrics passed")["status"],"APPROVED")
        with self.assertRaises(ValueError): transition("CANDIDATE","ACTIVE",actor="trainer",reason="automatic")
