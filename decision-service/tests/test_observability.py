import io
import json
import logging
import unittest
from logging_config import JsonFormatter

class ObservabilityTests(unittest.TestCase):
    def test_json_log_has_structured_correlation_without_secrets(self):
        record = logging.LogRecord("decision-service", logging.INFO, __file__, 1, "http_request", (), None)
        record.service = "decision-service"; record.event = "http_request"; record.correlation_id = "trace-123"; record.route = "/api/v1/predict/outcomes"
        payload = json.loads(JsonFormatter().format(record))
        self.assertEqual(payload["correlation_id"], "trace-123")
        rendered = json.dumps(payload)
        self.assertNotIn("authorization", rendered.lower())
        self.assertNotIn("contact", rendered.lower())
