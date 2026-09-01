import json,unittest
from datetime import datetime,timezone
from pathlib import Path
from features.pipeline import from_decision_context
from prediction.natural_service import NaturalPredictionRequest,default_path,load,predict_natural
from tests.test_prediction import PredictionTests
class NaturalPredictionTests(unittest.TestCase):
    def context(self):return PredictionTests().context()
    def test_artifact_probability_and_versions(self):
        bundle=load(str(default_path()));self.assertEqual(bundle["model_version"],"natural-recovery-v1")
        result=predict_natural(NaturalPredictionRequest(context=self.context()),now=datetime(2026,8,31,tzinfo=timezone.utc));self.assertTrue(0<=result.natural_recovery_probability<=1);self.assertEqual(result.feature_version,"natural-features-v1")
        self.assertEqual(round(result.natural_recovery_probability,8),0.22076675)
    def test_hidden_features_rejected(self):
        c=self.context();c["churn_intent"]=.9
        with self.assertRaises(ValueError):from_decision_context(c,"WAIT")
    def test_metadata_test_isolation(self):
        metadata=json.loads((Path(__file__).parents[1]/"models"/"natural_recovery_v1_metadata.json").read_text());self.assertFalse(metadata["held_out_test_used"])
if __name__=="__main__":unittest.main()
