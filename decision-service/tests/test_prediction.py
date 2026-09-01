import json
import unittest
from datetime import datetime, timezone
from pathlib import Path

from fastapi.testclient import TestClient

from features.pipeline import FEATURE_NAMES, from_decision_context
from main import app
from prediction.service import PredictionRequest, default_model_path, load_bundle, predict


class PredictionTests(unittest.TestCase):
    def context(self):
        return {
            "feature_version": "recovery-context-v1",
            "case": {"case_id":"case-1","leak_type":"FAILED_SUBSCRIPTION","amount_at_risk_minor":849900,"failure_or_leak_context":{}},
            "diagnosis": {"failure_category":"INSUFFICIENT_FUNDS","recoverability":"TEMPORARY","confidence":0.9},
            "customer_profile": {"successful_payment_ratio":0.91,"successful_payment_count":20,"recent_failures":2,"contact_count":0,"promise_reliability":0.8,"recovery_fatigue":0.1,"subscription_tenure_days":720,"preferred_payment_method":"UPI"},
            "merchant_context": {"merchant_type":"SAAS"},
            "recent_actions": [],
            "timing_context": {"hours_since_leak":4},
            "payment_state": {},
        }

    def test_artifact_load_and_probability_contract(self):
        bundle = load_bundle(str(default_model_path()))
        self.assertEqual(bundle["model_version"], "outcome-v1")
        request = PredictionRequest(context=self.context(), eligible_actions=["WAIT","RETRY_LATER"])
        response = predict(request, now=datetime(2026,8,31,tzinfo=timezone.utc))
        self.assertEqual([item.action for item in response.predictions], ["WAIT","RETRY_LATER"])
        self.assertTrue(all(0 <= item.recovery_probability <= 1 for item in response.predictions))
        self.assertTrue(all(item.model_version == "outcome-v1" and item.feature_version == "features-v1" for item in response.predictions))

    def test_excluded_action_is_not_predicted(self):
        response = predict(PredictionRequest(context=self.context(), eligible_actions=["WAIT"]))
        self.assertEqual([item.action for item in response.predictions], ["WAIT"])

    def test_frozen_fixture_probabilities(self):
        response = predict(PredictionRequest(context=self.context(), eligible_actions=["WAIT", "RETRY_LATER"]))
        self.assertEqual(
            [round(item.recovery_probability, 8) for item in response.predictions],
            [0.18865290, 0.48342913],
        )

    def test_hidden_feature_leakage_rejected(self):
        context = self.context();context["liquidity_pattern"] = "STABLE"
        with self.assertRaises(ValueError):
            from_decision_context(context,"WAIT")

    def test_preprocessing_is_deterministic_and_versioned(self):
        left=from_decision_context(self.context(),"WAIT");right=from_decision_context(self.context(),"WAIT")
        self.assertEqual(left,right);self.assertEqual(len(left),len(FEATURE_NAMES))

    def test_http_schema_validation(self):
        client=TestClient(app);response=client.post("/api/v1/predict/outcomes",json={"context":self.context(),"eligible_actions":["INVENTED_ACTION"]})
        self.assertEqual(response.status_code,422)

    def test_training_metadata_proves_test_isolation(self):
        metadata=json.loads((Path(__file__).parents[1]/"models"/"outcome_v1_metadata.json").read_text(encoding="utf-8"))
        self.assertFalse(metadata["held_out_test_used"]);self.assertNotIn("test",metadata["training_dataset"]["path"].lower())

if __name__=="__main__":unittest.main()
